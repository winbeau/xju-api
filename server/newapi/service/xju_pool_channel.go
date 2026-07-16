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

// renamePoolGroup renames a pool's routing channel — both its display name AND
// its group (the card-routing key) — to newLabel. Because the group is a routing
// key, every already-issued card in the old group, plus the GroupRatio /
// UserUsableGroups entries, is migrated to the new group so routing and billing
// stay intact. It refuses if the new name is already used by another group so a
// rename can't silently merge two pools' cards.
func renamePoolGroup(poolID string, channelID int, newLabel string) error {
	ch := poolChannel(poolID, channelID)
	if ch == nil {
		return fmt.Errorf("routing channel not found for pool %s", poolID)
	}
	oldGroup := ch.Group
	newGroup := newLabel

	if oldGroup != newGroup && groupInUse(newGroup, oldGroup) {
		return fmt.Errorf("the name %q is already used by another group; pick a different name or remove that group first", newGroup)
	}

	// Channel.Update() rebuilds this channel's abilities for the (possibly new)
	// group, so relay routing follows the rename.
	ch.Name = newLabel
	ch.Group = newGroup
	if err := ch.Update(); err != nil {
		return err
	}

	if oldGroup == newGroup {
		setPoolGroupLabel(newGroup, newLabel) // only the display label changed
		return nil
	}

	// Migrate already-issued cards so their routing survives the group rename.
	if err := model.DB.Model(&model.Token{}).
		Where(&model.Token{Group: oldGroup}).
		Update("group", newGroup).Error; err != nil {
		common.SysError("pool rename: token group migration " + oldGroup + "->" + newGroup + " failed: " + err.Error())
	}
	migratePoolGroupOptions(oldGroup, newGroup, newLabel)
	return nil
}

// poolChannel resolves a pool's routing channel by id, falling back to its
// cliproxy-pool-<id> name.
func poolChannel(poolID string, channelID int) *model.Channel {
	if channelID != 0 {
		if ch, err := model.GetChannelById(channelID, false); err == nil && ch != nil {
			return ch
		}
	}
	if id := findChannelByName(poolChannelName(poolID)); id != 0 {
		if ch, err := model.GetChannelById(id, false); err == nil {
			return ch
		}
	}
	return nil
}

// groupInUse reports whether `group` is already registered — as a channel group,
// a GroupRatio key, or a UserUsableGroups key — ignoring the pool's own current
// group. Used to refuse a rename that would collide two groups.
func groupInUse(group, exceptGroup string) bool {
	if strings.TrimSpace(group) == "" || group == exceptGroup {
		return false
	}
	if _, ok := setting.GetUserUsableGroupsCopy()[group]; ok {
		return true
	}
	if _, ok := ratio_setting.GetGroupRatioCopy()[group]; ok {
		return true
	}
	var ch model.Channel
	if err := model.DB.Where(&model.Channel{Group: group}).Select("id").First(&ch).Error; err == nil && ch.Id != 0 {
		return true
	}
	return false
}

// migratePoolGroupOptions moves the GroupRatio value and UserUsableGroups entry
// from oldGroup to newGroup (labelled newLabel), preserving the ratio.
func migratePoolGroupOptions(oldGroup, newGroup, newLabel string) {
	gr := ratio_setting.GetGroupRatioCopy()
	if gr == nil {
		gr = map[string]float64{}
	}
	if v, ok := gr[oldGroup]; ok {
		gr[newGroup] = v
		delete(gr, oldGroup)
	} else if _, ok := gr[newGroup]; !ok {
		gr[newGroup] = ratio_setting.GetGroupRatio("default")
	}
	if b, err := common.Marshal(gr); err == nil {
		_ = model.UpdateOption("GroupRatio", string(b))
	}
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	delete(uug, oldGroup)
	uug[newGroup] = newLabel
	if b, err := common.Marshal(uug); err == nil {
		_ = model.UpdateOption("UserUsableGroups", string(b))
	}
}

// setPoolGroupLabel refreshes only a group's display label (its key is unchanged).
func setPoolGroupLabel(group, label string) {
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	if uug[group] != label {
		uug[group] = label
		if b, err := common.Marshal(uug); err == nil {
			_ = model.UpdateOption("UserUsableGroups", string(b))
		}
	}
}
