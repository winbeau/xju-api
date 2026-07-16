package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// xju-api:new — pool routing channel (#4 Phase C). A dynamically-created pool is
// only reachable for relay once a new-api channel routes its group to the pool's
// cliproxy instance. This mirrors scripts/create-k12-channel.sh in Go: clone an
// existing pool channel's model set, create an OpenAI-compatible channel keyed by
// the pool's internal relay key, and register the group in the ratio/usable-group
// options so cards in that group bill and route correctly.

const poolChannelType = 1 // OpenAI-compatible

func poolChannelName(poolID string) string { return "cliproxy-pool-" + poolID }

// findChannelByName returns a channel id by exact name, or 0 if none exists.
func findChannelByName(name string) int {
	var ch model.Channel
	if err := model.DB.Where("name = ?", name).Select("id").First(&ch).Error; err != nil {
		return 0
	}
	return ch.Id
}

// poolTemplateModels returns the model set to seed a new pool channel with. Every
// cliproxy pool shares the same model set, so it clones from any existing pool
// channel. It resolves the channel by a registered pool's channel id first — that
// survives a channel rename (which moves the name away from the cliproxy-pool
// prefix) — and falls back to the name prefix for any channel not yet in the
// registry. This replaces the old hard dependency on channel id 1.
func poolTemplateModels() (string, error) {
	// Primary: a registered pool's channel, resolved by id (rename-proof).
	for _, p := range common.ListConfiguredPools() {
		entry, ok := common.GetPoolEntry(p.ID)
		if !ok || entry.ChannelID == 0 {
			continue
		}
		if ch, err := model.GetChannelById(entry.ChannelID, false); err == nil && ch != nil && strings.TrimSpace(ch.Models) != "" {
			return ch.Models, nil
		}
	}
	// Fallback: any channel still using the cliproxy-pool name prefix.
	var ch model.Channel
	if err := model.DB.
		Where("type = ? AND name LIKE ? AND models <> ''", poolChannelType, "cliproxy-pool%").
		Order("id").
		Select("models").
		First(&ch).Error; err != nil {
		return "", fmt.Errorf("no existing cliproxy pool channel to clone models from: %w", err)
	}
	return ch.Models, nil
}

// createPoolChannel creates the routing channel for a pool (idempotent by name)
// and registers its group. Returns the channel id.
func createPoolChannel(poolID, internalKey, baseURL, label string) (int, error) {
	models, err := poolTemplateModels()
	if err != nil {
		return 0, err
	}
	name := poolChannelName(poolID)
	if existing := findChannelByName(name); existing != 0 {
		return existing, nil
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	ch := &model.Channel{
		Type:    poolChannelType,
		Name:    name,
		Key:     internalKey,
		BaseURL: &base,
		Models:  models,
		Group:   poolID,
		Status:  1, // enabled
	}
	if err := ch.Insert(); err != nil {
		return 0, err
	}
	// Group options are non-critical: log but don't fail the channel on error.
	if err := addPoolGroupOptions(poolID, label); err != nil {
		common.SysError("pool channel: group options update failed for " + poolID + ": " + err.Error())
	}
	return ch.Id, nil
}

// deletePoolChannel removes a pool's routing channel + its group registration.
// Best-effort: it logs failures rather than blocking pool deletion.
func deletePoolChannel(poolID string, channelID int) {
	if channelID == 0 {
		channelID = findChannelByName(poolChannelName(poolID))
	}
	if channelID != 0 {
		ch := model.Channel{Id: channelID}
		if err := ch.Delete(); err != nil {
			common.SysError("pool channel delete failed for " + poolID + ": " + err.Error())
		}
	}
	removePoolGroupOptions(poolID)
}

// addPoolGroupOptions registers the pool's group in GroupRatio (at the default
// ratio) and UserUsableGroups (labelled), matching the k12 setup.
func addPoolGroupOptions(poolID, label string) error {
	gr := ratio_setting.GetGroupRatioCopy()
	if gr == nil {
		gr = map[string]float64{}
	}
	if _, ok := gr[poolID]; !ok {
		gr[poolID] = ratio_setting.GetGroupRatio("default")
		b, err := common.Marshal(gr)
		if err != nil {
			return err
		}
		if err := model.UpdateOption("GroupRatio", string(b)); err != nil {
			return err
		}
	}
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	if _, ok := uug[poolID]; !ok {
		if strings.TrimSpace(label) == "" {
			label = poolID
		}
		uug[poolID] = label
		b, err := common.Marshal(uug)
		if err != nil {
			return err
		}
		if err := model.UpdateOption("UserUsableGroups", string(b)); err != nil {
			return err
		}
	}
	return nil
}

func removePoolGroupOptions(poolID string) {
	gr := ratio_setting.GetGroupRatioCopy()
	if _, ok := gr[poolID]; ok {
		delete(gr, poolID)
		if b, err := common.Marshal(gr); err == nil {
			_ = model.UpdateOption("GroupRatio", string(b))
		}
	}
	uug := setting.GetUserUsableGroupsCopy()
	if _, ok := uug[poolID]; ok {
		delete(uug, poolID)
		if b, err := common.Marshal(uug); err == nil {
			_ = model.UpdateOption("UserUsableGroups", string(b))
		}
	}
}

// renamePoolChannel updates a pool's routing-channel display name and its group's
// usable-group label to the new pool label — the operator-facing names. It is
// best-effort: relay routes off the group id, not these names, so failures are
// logged rather than propagated (the pool is already renamed in the registry).
func renamePoolChannel(poolID string, channelID int, newLabel string) {
	if channelID == 0 {
		channelID = findChannelByName(poolChannelName(poolID))
	}
	if channelID != 0 {
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("name", newLabel).Error; err != nil {
			common.SysError("pool rename: channel name update failed for " + poolID + ": " + err.Error())
		}
	}
	// Refresh the group's display label (shown when issuing cards); its ratio
	// entry in GroupRatio is left untouched.
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	if uug[poolID] != newLabel {
		uug[poolID] = newLabel
		if b, err := common.Marshal(uug); err == nil {
			if err := model.UpdateOption("UserUsableGroups", string(b)); err != nil {
				common.SysError("pool rename: usable-group label update failed for " + poolID + ": " + err.Error())
			}
		}
	}
}
