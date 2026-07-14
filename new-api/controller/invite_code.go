package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const maxInviteCodeBatch = 200

// GetInviteCodes lists invite codes with optional keyword/status filtering and
// pagination — the single endpoint that backs the management table.
func GetInviteCodes(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.SearchInviteCodes(keyword, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

// GenerateInviteCodes creates a batch of single-use invite codes. Body accepts
// {count, valid_days}: count defaults to 1, valid_days 0 means never expires.
func GenerateInviteCodes(c *gin.Context) {
	body := model.InviteCode{}
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiError(c, err)
		return
	}
	count := body.Count
	if count <= 0 {
		count = 1
	}
	if count > maxInviteCodeBatch {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "too many codes requested at once (max " + strconv.Itoa(maxInviteCodeBatch) + ")",
		})
		return
	}

	var expiredTime int64
	if body.ValidDays > 0 {
		expiredTime = common.GetTimestamp() + int64(body.ValidDays)*86400
	}

	creatorId := c.GetInt("id")
	now := common.GetTimestamp()
	var codes []string
	for i := 0; i < count; i++ {
		code := common.GetUUID()
		clean := model.InviteCode{
			Code:        code,
			Status:      common.InviteCodeStatusEnabled,
			CreatorId:   creatorId,
			CreatedTime: now,
			ExpiredTime: expiredTime,
		}
		if err := clean.Insert(); err != nil {
			common.SysError("failed to insert invite code: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "failed to create invite codes",
				"data":    codes,
			})
			return
		}
		codes = append(codes, code)
	}

	recordManageAudit(c, "invite_code.create", map[string]interface{}{
		"count":     count,
		"valid_days": body.ValidDays,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    codes,
	})
}

// UpdateInviteCodeStatus enables or disables a code. Body: {id, status}.
func UpdateInviteCodeStatus(c *gin.Context) {
	body := struct {
		Id     int `json:"id"`
		Status int `json:"status"`
	}{}
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiError(c, err)
		return
	}
	if body.Status != common.InviteCodeStatusEnabled && body.Status != common.InviteCodeStatusDisabled {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid status"})
		return
	}
	if err := model.SetInviteCodeStatus(body.Id, body.Status); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invite_code.status", map[string]interface{}{"id": body.Id, "status": body.Status})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func DeleteInviteCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteInviteCodeById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invite_code.delete", map[string]interface{}{"id": id})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// DeleteInvalidInviteCodes prunes used/disabled/expired codes in one shot.
func DeleteInvalidInviteCodes(c *gin.Context) {
	n, err := model.DeleteInvalidInviteCodes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invite_code.prune", map[string]interface{}{"deleted": n})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": n})
}
