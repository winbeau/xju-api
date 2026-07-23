package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareResponsesWebsocketRequestPreservesTransportFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Set("original_model", "gpt-5.6-sol")
	c.Set("model_mapping", `{"gpt-5.6-sol":"gpt-5.6-terra"}`)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "http://cli-proxy-api:8317")
	common.SetContextKey(c, constant.ContextKeyChannelKey, "__INTERNAL_KEY__")

	generate := false
	request := &dto.OpenAIResponsesRequest{
		Model:    "gpt-5.6-sol",
		Input:    []byte("[]"),
		Generate: &generate,
	}
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIResponses, request, nil)
	require.NoError(t, err)

	payload, apiErr := PrepareResponsesWebsocketRequest(
		c,
		info,
		request,
		responsesWebsocketRequestCreateForTest,
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","generate":false,"input":[]}`),
	)
	require.Nil(t, apiErr)
	assert.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
	assert.Equal(t, "gpt-5.6-terra", gjson.GetBytes(payload, "model").String())
	assert.True(t, gjson.GetBytes(payload, "generate").Exists())
	assert.False(t, gjson.GetBytes(payload, "generate").Bool())
}

const responsesWebsocketRequestCreateForTest = "response.create"
