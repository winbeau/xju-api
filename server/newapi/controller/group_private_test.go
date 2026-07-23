package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserGroupsAddsOnlyOwnedPrivateGroup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "alice", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", Label: "Alice Pool", MgmtURL: "http://pool-1:8319", MgmtSecret: "secret", ChannelID: 123,
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	oldRatios := ratio_setting.GroupRatio2JSONString()
	ratios := ratio_setting.GetGroupRatioCopy()
	ratios[common.PrivatePoolGroupKey(42)] = 1
	rawRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(rawRatios)))
	t.Cleanup(func() { _ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios) })

	_, globallyVisible := setting.GetUserUsableGroupsCopy()[common.PrivatePoolGroupKey(42)]
	assert.False(t, globallyVisible)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)
	c.Set("id", 42)
	GetUserGroups(c)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Success bool                              `json:"success"`
		Data    map[string]map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	privateGroup, ok := response.Data[common.PrivatePoolGroupKey(42)]
	require.True(t, ok)
	assert.Equal(t, "Alice Pool", privateGroup["desc"])
}
