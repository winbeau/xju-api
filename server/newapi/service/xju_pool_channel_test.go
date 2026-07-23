package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — pool routing channel (#4 Phase C). A provisioned pool gets an
// auto-created channel that clones the primary channel's models and routes the
// pool's group to its cliproxy instance; deleting the pool removes it.

func TestPoolChannelCreateDelete(t *testing.T) {
	// Seed the primary channel (id 1) whose model set is cloned.
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Type: 1, Name: "cliproxy-pool", Key: "k1",
		Models: "gpt-5,gpt-4", Group: "default", Status: 1,
	}).Error)

	id, err := createPoolChannel("edu", "edu-key", "http://cli-proxy-api-edu:8319/", "Edu", "edu", true)
	require.NoError(t, err)
	require.NotZero(t, id)

	var ch model.Channel
	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, "cliproxy-pool-edu", ch.Name)
	assert.Equal(t, "edu-key", ch.Key)
	assert.Equal(t, "edu", ch.Group)
	assert.Equal(t, "gpt-5,gpt-4", ch.Models, "models cloned from channel 1")
	require.NotNil(t, ch.BaseURL)
	assert.Equal(t, "http://cli-proxy-api-edu:8319", *ch.BaseURL, "trailing slash trimmed")

	// Idempotent by name — a second create returns the same channel.
	id2, err := createPoolChannel("edu", "ignored", "ignored", "Edu", "edu", true)
	require.NoError(t, err)
	assert.Equal(t, id, id2)

	// findChannelByName resolves it; a bogus name resolves to 0.
	assert.Equal(t, id, findChannelByName("cliproxy-pool-edu"))
	assert.Zero(t, findChannelByName("cliproxy-pool-nope"))

	// Delete removes the channel row.
	deletePoolChannel("edu", "edu", id)
	var cnt int64
	model.DB.Model(&model.Channel{}).Where("id = ?", id).Count(&cnt)
	assert.Zero(t, cnt, "channel deleted")
}

func TestPrivatePoolChannelUsesHiddenImmutableGroup(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Type: 1, Name: "cliproxy-pool", Key: "k1",
		Models: "gpt-5", Group: "default", Status: 1,
	}).Error)

	groupKey := common.PrivatePoolGroupKey(4242)
	id, err := createPoolChannel("private-case", "private-key", "http://private:9000", "Alice Pool", groupKey, false)
	require.NoError(t, err)
	t.Cleanup(func() { deletePoolChannel("private-case", groupKey, id) })

	var ch model.Channel
	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, groupKey, ch.Group)
	assert.True(t, ratio_setting.ContainsGroupRatio(groupKey), "private group still needs a billing ratio")
	_, globallyVisible := setting.GetUserUsableGroupsCopy()[groupKey]
	assert.False(t, globallyVisible, "private group must not be exposed globally")

	card := &model.Token{UserId: 4242, Key: "privategroupkey000000000000000000000001", Name: "private", Group: groupKey}
	require.NoError(t, model.DB.Create(card).Error)
	require.NoError(t, renamePoolGroup("private-case", id, "Renamed Pool", groupKey, false))

	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, "Renamed Pool", ch.Name)
	assert.Equal(t, groupKey, ch.Group, "renaming display label cannot change private routing")
	var cardAfter model.Token
	require.NoError(t, model.DB.First(&cardAfter, card.Id).Error)
	assert.Equal(t, groupKey, cardAfter.Group)
	_, globallyVisible = setting.GetUserUsableGroupsCopy()[groupKey]
	assert.False(t, globallyVisible)
}
