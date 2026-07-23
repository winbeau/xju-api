package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ReconcilePoolChannelCompatibility upgrades every known CLIProxyAPI routing
// channel in place. Registered channel ids make this rename-proof; the name
// fallback also covers the built-in/template channels and legacy entries that
// predate registry channel ids. Failures are isolated per channel so one broken
// row cannot prevent New API from starting.
func ReconcilePoolChannelCompatibility() (upgraded int, failed int) {
	channels, err := registeredPoolChannels()
	if err != nil {
		common.SysError("pool channel compatibility discovery failed: " + err.Error())
		return 0, 1
	}

	for _, channel := range channels {
		changed, err := ensurePoolChannelCompatibility(channel)
		if err != nil {
			failed++
			common.SysError(fmt.Sprintf("pool channel compatibility failed: channel_id=%d: %v", channel.Id, err))
			continue
		}
		if changed {
			upgraded++
			common.SysLog(fmt.Sprintf("pool channel compatibility upgraded: channel_id=%d", channel.Id))
		}
	}
	return upgraded, failed
}

func registeredPoolChannels() ([]*model.Channel, error) {
	byID := make(map[int]*model.Channel)
	for _, channelID := range common.ListRegisteredPoolChannelIDs() {
		channel, err := model.GetChannelById(channelID, false)
		if err != nil || channel == nil {
			continue
		}
		byID[channel.Id] = channel
	}

	var named []*model.Channel
	if err := model.DB.
		Where("name = ? OR name LIKE ?", "cliproxy-pool", "cliproxy-pool-%").
		Find(&named).Error; err != nil {
		return nil, err
	}
	for _, channel := range named {
		if channel != nil && channel.Id > 0 {
			byID[channel.Id] = channel
		}
	}

	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	channels := make([]*model.Channel, 0, len(ids))
	for _, id := range ids {
		channels = append(channels, byID[id])
	}
	return channels, nil
}

func ensurePoolChannelCompatibility(channel *model.Channel) (bool, error) {
	if channel == nil || channel.Id <= 0 {
		return false, fmt.Errorf("invalid channel")
	}

	beforeType := channel.Type
	beforeSetting := stringValue(channel.Setting)
	beforeOtherSettings := channel.OtherSettings
	beforeHeaderOverride := stringValue(channel.HeaderOverride)

	applyPoolChannelCompatibility(channel)
	if err := channel.ValidateSettings(); err != nil {
		return false, err
	}

	changed := beforeType != channel.Type ||
		beforeSetting != stringValue(channel.Setting) ||
		beforeOtherSettings != channel.OtherSettings ||
		beforeHeaderOverride != stringValue(channel.HeaderOverride)
	if !changed {
		return false, nil
	}
	if err := channel.Update(); err != nil {
		return false, err
	}
	return true, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
