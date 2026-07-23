package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetRelayRouterRegistersResponsesWebsocket(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	found := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/responses" {
			found = true
			break
		}
	}
	assert.True(t, found, "GET /v1/responses must be registered for Codex WebSocket mode")
}
