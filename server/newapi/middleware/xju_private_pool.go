package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// PrivatePoolScope binds management handlers to the authenticated user's own
// registered private pool. It intentionally ignores every pool id supplied by
// the request, so guessing another pool's numeric id cannot cross the boundary.
func PrivatePoolScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerUserID := c.GetInt("id")
		entry, ok := common.FindPrivatePoolByOwner(ownerUserID)
		if !ok {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "private pool is required",
			})
			c.Abort()
			return
		}
		if _, _, ready := common.ResolvePoolMgmt(entry.ID); !ready {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "private pool is not ready",
			})
			c.Abort()
			return
		}
		c.Set(common.ContextKeyPrivatePoolID, entry.ID)
		c.Set(common.ContextKeyPrivatePoolScope, true)
		c.Next()
	}
}
