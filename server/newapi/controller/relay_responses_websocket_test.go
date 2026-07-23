package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponsesWebsocketRequest(t *testing.T) {
	t.Parallel()

	eventType, request, apiErr := parseResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","generate":false}`),
		"",
	)
	require.Nil(t, apiErr)
	assert.Equal(t, responsesWebsocketRequestCreate, eventType)
	assert.Equal(t, "gpt-5.6-sol", request.Model)
	require.NotNil(t, request.Generate)
	assert.False(t, *request.Generate)
	assert.JSONEq(t, "[]", string(request.Input))

	_, request, apiErr = parseResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","previous_response_id":"resp_1","input":[]}`),
		"gpt-5.6-sol",
	)
	require.Nil(t, apiErr)
	assert.Equal(t, "gpt-5.6-sol", request.Model)

	_, _, apiErr = parseResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","model":"gpt-5.5","input":[]}`),
		"gpt-5.6-sol",
	)
	require.NotNil(t, apiErr)
	assert.Equal(t, 400, apiErr.StatusCode)
}

func TestResponsesWebsocketTurnObserveCompletedUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	turn := &responsesWebsocketTurn{
		info: &relaycommon.RelayInfo{
			ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
				BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
					dto.BuildInToolWebSearchPreview: {ToolName: dto.BuildInToolWebSearchPreview},
				},
			},
		},
	}

	terminal := turn.observe(nil, []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`))
	assert.Equal(t, responsesWebsocketTerminalNone, terminal)
	assert.Equal(t, 1, turn.info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)

	terminal = turn.observe(nil, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":120,"output_tokens":30,"total_tokens":150,"input_tokens_details":{"cached_tokens":20}}}}`))
	assert.Equal(t, responsesWebsocketTerminalCompleted, terminal)
	assert.Equal(t, 120, turn.usage.PromptTokens)
	assert.Equal(t, 30, turn.usage.CompletionTokens)
	assert.Equal(t, 150, turn.usage.TotalTokens)
	assert.Equal(t, 20, turn.usage.PromptTokensDetails.CachedTokens)
}

func TestResponsesWebsocketTurnObserveTerminalEvents(t *testing.T) {
	t.Parallel()

	turn := &responsesWebsocketTurn{info: &relaycommon.RelayInfo{}}
	terminal := turn.observe(nil, []byte(`{"type":"response.incomplete","response":{"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10}}}`))
	assert.Equal(t, responsesWebsocketTerminalCompleted, terminal)
	assert.Equal(t, 10, turn.usage.TotalTokens)

	for _, eventType := range []string{"error", "response.error", "response.failed"} {
		turn := &responsesWebsocketTurn{info: &relaycommon.RelayInfo{}}
		terminal = turn.observe(nil, []byte(`{"type":"`+eventType+`"}`))
		assert.Equal(t, responsesWebsocketTerminalError, terminal, eventType)
	}
}

func TestResponsesWebsocketTurnObservesInheritedWebSearchTool(t *testing.T) {
	t.Parallel()

	turn := &responsesWebsocketTurn{
		info: &relaycommon.RelayInfo{
			ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
				BuiltInTools: map[string]*relaycommon.BuildInToolInfo{},
			},
		},
	}

	terminal := turn.observe(nil, []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`))
	assert.Equal(t, responsesWebsocketTerminalNone, terminal)
	tool := turn.info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]
	require.NotNil(t, tool)
	assert.Equal(t, 1, tool.CallCount)
}
