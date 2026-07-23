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

func TestSetRelayRouterRegistersClaudeCountTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	found := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/messages/count_tokens" {
			found = true
			break
		}
	}
	assert.True(t, found, "POST /v1/messages/count_tokens must be registered")
}
