package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivatePoolScopeBindsAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", Label: "Alice", MgmtURL: "http://pool-1:8319", MgmtSecret: "secret",
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 42)
		c.Next()
	})
	router.Use(PrivatePoolScope())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"pool_id": c.GetString(common.ContextKeyPrivatePoolID),
			"scoped":  c.GetBool(common.ContextKeyPrivatePoolScope),
		})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test?pool=999", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"pool_id":"1","scoped":true}`, w.Body.String())
}

func TestPrivatePoolScopeRejectsUserWithoutPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 7)
		c.Next()
	})
	router.Use(PrivatePoolScope())
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestValidateTokenPrivatePoolGroupOwnership(t *testing.T) {
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", MgmtURL: "http://pool-1:8319", MgmtSecret: "s", ChannelID: 101,
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "2", MgmtURL: "http://pool-2:8320", MgmtSecret: "s", ChannelID: 102,
		OwnerUserID: 7, Kind: common.PoolKindPrivate,
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "3", MgmtURL: "http://pool-3:8321", MgmtSecret: "s", ChannelID: 0,
		OwnerUserID: 9, Kind: common.PoolKindPrivate,
	}))

	private, err := validateTokenPrivatePoolGroup(42, "private-42")
	assert.True(t, private)
	assert.NoError(t, err)

	private, err = validateTokenPrivatePoolGroup(42, "private-7")
	assert.True(t, private)
	assert.Error(t, err, "user cannot use another owner's private group")

	private, err = validateTokenPrivatePoolGroup(42, "private-999")
	assert.True(t, private)
	assert.Error(t, err, "unknown private group is rejected")

	private, err = validateTokenPrivatePoolGroup(9, "private-9")
	assert.True(t, private)
	assert.ErrorIs(t, err, errPrivatePoolUnavailable)

	private, err = validateTokenPrivatePoolGroup(42, "default")
	assert.False(t, private)
	assert.NoError(t, err)
}

func TestSetupContextDisablesPrivateCrossGroupRetryAndSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", MgmtURL: "http://pool-1:8319", MgmtSecret: "s", ChannelID: 101,
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))
	token := &model.Token{Id: 1, UserId: 42, Key: "token", Group: "private-42", CrossGroupRetry: true, UnlimitedQuota: true}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.NoError(t, SetupContextForToken(c, token))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry))
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyPrivatePoolBalanceExempt))

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	err := SetupContextForToken(c, token, "token", "999")
	assert.Error(t, err)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
