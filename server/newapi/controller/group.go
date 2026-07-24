package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    getUserTokenGroups(c.GetInt("id")),
	})
}

// getUserTokenGroups is the single source of truth for API-key routing choices.
// XJU currently exposes one shared pool (default) plus the authenticated user's
// ready private pool. Historical ratio/UserUsableGroups entries such as k12 or
// vip must not leak back into the key editor after their pools are retired.
func getUserTokenGroups(userID int) map[string]map[string]interface{} {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userGroup, _ = model.GetUserGroup(userID, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	if desc, ok := userUsableGroups["default"]; ok && ratio_setting.ContainsGroupRatio("default") {
		usableGroups["default"] = map[string]interface{}{
			"ratio": service.GetUserGroupRatio(userGroup, "default"),
			"desc":  desc,
		}
	}
	// Private groups never live in global UserUsableGroups. Add exactly the
	// authenticated user's ready pool here so group discovery remains owner-bound.
	if entry, ok := common.FindPrivatePoolByOwner(userID); ok && entry.ChannelID > 0 {
		if _, _, ready := common.ResolvePoolMgmt(entry.ID); ready && ratio_setting.ContainsGroupRatio(entry.GroupKey) {
			desc := entry.Label
			if desc == "" {
				desc = "我的私人号池"
			}
			usableGroups[entry.GroupKey] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, entry.GroupKey),
				"desc":  desc,
			}
		}
	}
	return usableGroups
}

// resolveUserTokenGroup validates a requested API-key group and supplies the
// creation default: own private pool first, then the shared default pool.
func resolveUserTokenGroup(userID int, requested string) (string, bool) {
	groups := getUserTokenGroups(userID)
	requested = strings.TrimSpace(requested)
	if requested == "" {
		privateGroup := common.PrivatePoolGroupKey(userID)
		if _, ok := groups[privateGroup]; ok {
			return privateGroup, true
		}
		if _, ok := groups["default"]; ok {
			return "default", true
		}
		return "", false
	}
	_, ok := groups[requested]
	return requested, ok
}
