package channel_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoWssRequestConvertsHTTPURLAndForwardsSafeHeaders(t *testing.T) {
	t.Parallel()

	requestHeaders := make(chan http.Header, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex-test")
	c.Request.Header.Set("X-Codex-Turn-State", "turn-123")

	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RelayFormat:    types.RelayFormatOpenAIResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiType:           constant.APITypeOpenAI,
			ApiKey:            "__INTERNAL_KEY__",
			UpstreamModelName: "gpt-5.6-sol",
		},
	}

	adaptor := &openai.Adaptor{}
	adaptor.Init(info)
	conn, err := channel.DoWssRequest(adaptor, c, info, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	headers := <-requestHeaders
	assert.Equal(t, "Bearer __INTERNAL_KEY__", headers.Get("Authorization"))
	assert.Equal(t, "codex-test", headers.Get("User-Agent"))
	assert.Equal(t, "turn-123", headers.Get("X-Codex-Turn-State"))
}
