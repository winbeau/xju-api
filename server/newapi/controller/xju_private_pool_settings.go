package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

type privatePoolSettingsPatch struct {
	AutoCleanEnabled        *bool `json:"auto_clean_enabled"`
	AutoCleanHours          *int  `json:"auto_clean_hours"`
	UsageAutoRefreshEnabled *bool `json:"usage_auto_refresh_enabled"`
	UsageAutoResetEnabled   *bool `json:"usage_auto_reset_enabled"`
}

func privatePoolSettingsFromEntry(entry common.PoolEntry) common.PrivatePoolSettings {
	hours := entry.AutoCleanHours
	if hours <= 0 {
		hours = 24
	}
	return common.PrivatePoolSettings{
		AutoCleanEnabled:        entry.AutoCleanEnabled,
		AutoCleanHours:          hours,
		UsageAutoRefreshEnabled: entry.UsageAutoRefreshEnabled,
		UsageAutoResetEnabled:   entry.UsageAutoResetEnabled,
	}
}

func GetPrivatePoolSettings(c *gin.Context) {
	entry, ok := common.FindPrivatePoolByOwner(c.GetInt("id"))
	if !ok || entry.ID != poolIDFromRequest(c) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "private pool not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": privatePoolSettingsFromEntry(entry)})
}

func PatchPrivatePoolSettings(c *gin.Context) {
	ownerUserID := c.GetInt("id")
	entry, ok := common.FindPrivatePoolByOwner(ownerUserID)
	if !ok || entry.ID != poolIDFromRequest(c) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "private pool not found"})
		return
	}
	var patch privatePoolSettingsPatch
	if err := common.DecodeJson(c.Request.Body, &patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	settings := privatePoolSettingsFromEntry(entry)
	if patch.AutoCleanEnabled != nil {
		settings.AutoCleanEnabled = *patch.AutoCleanEnabled
	}
	if patch.AutoCleanHours != nil {
		if *patch.AutoCleanHours < 1 || *patch.AutoCleanHours > 168 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "auto clean hours must be between 1 and 168"})
			return
		}
		settings.AutoCleanHours = *patch.AutoCleanHours
	}
	if patch.UsageAutoRefreshEnabled != nil {
		settings.UsageAutoRefreshEnabled = *patch.UsageAutoRefreshEnabled
	}
	if patch.UsageAutoResetEnabled != nil {
		settings.UsageAutoResetEnabled = *patch.UsageAutoResetEnabled
	}
	if err := common.SetPrivatePoolSettings(entry.ID, ownerUserID, settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, "private_pool.settings", map[string]interface{}{
		"pool":                       auditPoolID(entry.ID),
		"auto_clean_enabled":         settings.AutoCleanEnabled,
		"auto_clean_hours":           settings.AutoCleanHours,
		"usage_auto_refresh_enabled": settings.UsageAutoRefreshEnabled,
		"usage_auto_reset_enabled":   settings.UsageAutoResetEnabled,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}
