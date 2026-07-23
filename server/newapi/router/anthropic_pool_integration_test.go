package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedAnthropicRequest struct {
	Path          string
	Authorization string
	APIKey        string
	Beta          string
	SessionID     string
	Stainless     string
	UserAgent     string
	Body          []byte
}

func TestClaudeCountTokensPrivatePoolOwnerIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	requestCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		assert.Equal(t, "Bearer private-internal-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":11}`))
	}))
	defer upstream.Close()

	group := common.PrivatePoolGroupKey(2)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(`{"default":1,%q:1}`, group)))
	channelID := seedAdvancedPoolChannel(t, upstream.URL, group, "private-internal-key", "Alice private pool")
	registryPath := filepath.Join(t.TempDir(), "pool-registry.json")
	t.Setenv("POOL_REGISTRY_FILE", registryPath)
	require.NoError(t, os.WriteFile(registryPath, []byte("[]"), 0o600))
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "private-2", Label: "Alice", MgmtURL: upstream.URL, MgmtSecret: "management-secret",
		ChannelID: channelID, OwnerUserID: 2, Kind: common.PoolKindPrivate, GroupKey: group,
	}}))

	seedRelayUser(t, 2, "default", 0)
	seedRelayUser(t, 3, "default", 100_000)
	seedRelayToken(t, 2, "privateownertoken0000000000000000001", group, common.TokenStatusEnabled, -1)
	seedRelayToken(t, 3, "privateattacktoken000000000000000001", group, common.TokenStatusEnabled, -1)

	engine := gin.New()
	SetRelayRouter(engine)
	payload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"private"}]}`)

	ownerRecorder := httptest.NewRecorder()
	ownerRequest := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(payload))
	ownerRequest.Header.Set("Content-Type", "application/json")
	ownerRequest.Header.Set("Authorization", "Bearer sk-privateownertoken0000000000000000001")
	engine.ServeHTTP(ownerRecorder, ownerRequest)
	require.Equal(t, http.StatusOK, ownerRecorder.Code, ownerRecorder.Body.String())
	assert.JSONEq(t, `{"input_tokens":11}`, ownerRecorder.Body.String())

	attackerRecorder := httptest.NewRecorder()
	attackerRequest := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(payload))
	attackerRequest.Header.Set("Content-Type", "application/json")
	attackerRequest.Header.Set("Authorization", "Bearer sk-privateattacktoken000000000000000001")
	engine.ServeHTTP(attackerRecorder, attackerRequest)
	assert.Equal(t, http.StatusForbidden, attackerRecorder.Code)
	assert.Equal(t, 1, requestCount, "another user's private-pool token must never reach the pool")
}

func TestAnthropicMessagesPoolRelayPreservesToolsThinkingAndSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	captured := make(chan capturedAnthropicRequest, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedAnthropicRequest{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Body:          body,
		}
		if bytes.Contains(body, []byte(`"stream":true`)) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"gpt-5\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":2,\"output_tokens\":0}}}\n\n")
			_, _ = io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\n")
			_, _ = io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n")
			_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if bytes.Contains(body, []byte(`"tool_result"`)) {
			_, _ = io.WriteString(w, `{"id":"msg_2","type":"message","role":"assistant","model":"gpt-5","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"gpt-5","content":[{"type":"thinking","thinking":"inspect file","signature":"sig"},{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"PLAN.md"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":2}}`)
	}))
	defer upstream.Close()

	seedRelayUser(t, 10, "default", 2_000_000)
	seedAdvancedPoolChannel(t, upstream.URL, "default", "messages-internal-key", "cliproxy-pool")
	seedRelayToken(t, 10, "messagestoken00000000000000000000001", "default", common.TokenStatusEnabled, -1)

	engine := gin.New()
	SetRelayRouter(engine)
	firstPayload := []byte(`{"model":"gpt-5","max_tokens":256,"messages":[{"role":"user","content":"read the plan"}],"tools":[{"name":"read","description":"read file","input_schema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],"thinking":{"type":"enabled","budget_tokens":1024}}`)
	first := performRelayIntegrationRequest(engine, "/v1/messages", "sk-messagestoken00000000000000000000001", firstPayload)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"type":"thinking"`)
	assert.Contains(t, first.Body.String(), `"type":"tool_use"`)
	assert.Contains(t, first.Body.String(), `"stop_reason":"tool_use"`)

	secondPayload := []byte(`{"model":"gpt-5","max_tokens":256,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"inspect file","signature":"sig"},{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"PLAN.md"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"contents"}]}]}`)
	second := performRelayIntegrationRequest(engine, "/v1/messages", "sk-messagestoken00000000000000000000001", secondPayload)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"text":"done"`)

	streamPayload := []byte(`{"model":"gpt-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"stream"}]}`)
	stream := performRelayIntegrationRequest(engine, "/v1/messages", "sk-messagestoken00000000000000000000001", streamPayload)
	require.Equal(t, http.StatusOK, stream.Code, stream.Body.String())
	assert.Equal(t, "text/event-stream", stream.Header().Get("Content-Type"))
	for _, event := range []string{"message_start", "content_block_start", "content_block_delta", "message_delta", "message_stop"} {
		assert.Contains(t, stream.Body.String(), "event: "+event)
	}

	for _, wantBody := range [][]byte{firstPayload, secondPayload, streamPayload} {
		got := <-captured
		assert.Equal(t, "/v1/messages", got.Path)
		assert.Equal(t, "Bearer messages-internal-key", got.Authorization)
		assert.JSONEq(t, string(wantBody), string(got.Body), "New API must preserve native Anthropic request bodies")
	}

	var user model.User
	require.NoError(t, model.DB.First(&user, 10).Error)
	assert.Less(t, user.Quota, 2_000_000, "public-pool generation still consumes platform quota")
	assert.Greater(t, user.UsedQuota, 0, "public-pool generation remains accounted")
}

func TestAdvancedPoolOpenAIChatAndResponsesRegression(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	type capturedRequest struct {
		path string
		body []byte
	}
	captured := make(chan capturedRequest, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedRequest{path: r.URL.Path, body: body}
		assert.Equal(t, "Bearer openai-internal-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = io.WriteString(w, `{"id":"chatcmpl_1","object":"chat.completion","created":1784836000,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		case "/v1/responses":
			_, _ = io.WriteString(w, `{"id":"resp_1","object":"response","created_at":1784836000,"status":"completed","model":"gpt-5","output":[{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"OK","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	seedRelayUser(t, 20, "default", 2_000_000)
	seedAdvancedPoolChannel(t, upstream.URL, "default", "openai-internal-key", "cliproxy-pool")
	seedRelayToken(t, 20, "openaitoken0000000000000000000000001", "default", common.TokenStatusEnabled, -1)
	engine := gin.New()
	SetRelayRouter(engine)

	chatPayload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`)
	chat := performRelayIntegrationRequest(engine, "/v1/chat/completions", "sk-openaitoken0000000000000000000000001", chatPayload)
	require.Equal(t, http.StatusOK, chat.Code, chat.Body.String())
	assert.Contains(t, chat.Body.String(), `"content":"OK"`)

	responsesPayload := []byte(`{"model":"gpt-5","input":"hello"}`)
	responses := performRelayIntegrationRequest(engine, "/v1/responses", "sk-openaitoken0000000000000000000000001", responsesPayload)
	require.Equal(t, http.StatusOK, responses.Code, responses.Body.String())
	assert.Contains(t, responses.Body.String(), `"object":"response"`)

	for _, expected := range []capturedRequest{
		{path: "/v1/chat/completions", body: chatPayload},
		{path: "/v1/responses", body: responsesPayload},
	} {
		got := <-captured
		assert.Equal(t, expected.path, got.path)
		assert.JSONEq(t, string(expected.body), string(got.body))
	}
}

func performRelayIntegrationRequest(engine http.Handler, path string, token string, payload []byte) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Anthropic-Version", "2023-06-01")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestClaudeCountTokensAuthenticatedPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	captured := make(chan capturedAnthropicRequest, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedAnthropicRequest{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			APIKey:        r.Header.Get("x-api-key"),
			Beta:          r.Header.Get("Anthropic-Beta"),
			SessionID:     r.Header.Get("X-Claude-Code-Session-Id"),
			Stainless:     r.Header.Get("X-Stainless-Runtime"),
			UserAgent:     r.Header.Get("User-Agent"),
			Body:          body,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":7}`))
	}))
	defer upstream.Close()

	seedRelayUser(t, 1, "default", 100_000)
	channelID := seedAdvancedPoolChannel(t, upstream.URL, "default", "pool-internal-key", "cliproxy-pool")
	assert.Positive(t, channelID)
	seedRelayToken(t, 1, "bearertoken000000000000000000000001", "default", common.TokenStatusEnabled, -1)
	seedRelayToken(t, 1, "xapikeytoken00000000000000000000001", "default", common.TokenStatusEnabled, -1)
	seedRelayToken(t, 1, "disabledtoken0000000000000000000001", "default", common.TokenStatusDisabled, -1)
	seedRelayToken(t, 1, "expiredtoken00000000000000000000001", "default", common.TokenStatusEnabled, time.Now().Add(-time.Hour).Unix())

	engine := gin.New()
	SetRelayRouter(engine)
	payload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}],"tools":[{"name":"read","input_schema":{"type":"object"}}],"thinking":{"type":"enabled","budget_tokens":1024}}`)

	for _, testCase := range []struct {
		name   string
		header string
		value  string
	}{
		{name: "bearer", header: "Authorization", value: "Bearer sk-bearertoken000000000000000000000001"},
		{name: "x-api-key", header: "x-api-key", value: "sk-xapikeytoken00000000000000000000001"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(testCase.header, testCase.value)
			request.Header.Set("Anthropic-Version", "2023-06-01")
			request.Header.Set("Anthropic-Beta", "prompt-caching-2024-07-31")
			request.Header.Set("X-Claude-Code-Session-Id", "session-123")
			request.Header.Set("X-Stainless-Runtime", "node")
			request.Header.Set("User-Agent", "claude-cli/2.1.0")
			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.JSONEq(t, `{"input_tokens":7}`, recorder.Body.String())
			got := <-captured
			assert.Equal(t, "/v1/messages/count_tokens", got.Path)
			assert.Equal(t, "Bearer pool-internal-key", got.Authorization)
			assert.Empty(t, got.APIKey)
			assert.Equal(t, "prompt-caching-2024-07-31", got.Beta)
			assert.Equal(t, "session-123", got.SessionID)
			assert.Equal(t, "node", got.Stainless)
			assert.Equal(t, "claude-cli/2.1.0", got.UserAgent)
			assert.JSONEq(t, string(payload), string(got.Body))
		})
	}

	for _, testCase := range []struct {
		name  string
		token string
	}{
		{name: "invalid", token: "sk-not-a-real-token"},
		{name: "disabled", token: "sk-disabledtoken0000000000000000000001"},
		{name: "expired", token: "sk-expiredtoken00000000000000000000001"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+testCase.token)
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}

	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 100_000, user.Quota, "count_tokens must not consume generation quota")
	assert.Equal(t, 0, user.UsedQuota, "count_tokens must not record generated-token usage")
	assert.Len(t, captured, 0, "rejected tokens must not reach CLIProxyAPI")
}

func setupRelayIntegrationDB(t *testing.T) {
	t.Helper()
	originalSQLitePath := common.SQLitePath
	originalMaster := common.IsMasterNode
	common.SQLitePath = filepath.Join(t.TempDir(), "relay-integration.db") + "?_busy_timeout=30000"
	common.IsMasterNode = false
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	require.NoError(t, model.DB.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{},
		&model.UserSubscription{}, &model.Option{},
	))
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	constant.StreamingTimeout = 300
	ratio_setting.InitRatioSettings()
	service.InitHttpClient()
	t.Cleanup(func() {
		common.SQLitePath = originalSQLitePath
		common.IsMasterNode = originalMaster
		sqlDB, sqlErr := model.DB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func seedRelayUser(t *testing.T, id int, group string, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id: id, Username: fmt.Sprintf("user-%d", id), Password: "password", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: group, Quota: quota, AffCode: fmt.Sprintf("aff-%d", id),
	}).Error)
}

func seedRelayToken(t *testing.T, userID int, key string, group string, status int, expiredTime int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: userID, Key: key, Name: key[:8], Group: group, Status: status,
		ExpiredTime: expiredTime, UnlimitedQuota: true,
	}).Error)
}

func seedAdvancedPoolChannel(t *testing.T, baseURL string, group string, key string, name string) int {
	t.Helper()
	headerOverride, err := json.Marshal(map[string]any{
		`re:(?i)^(anthropic-|x-claude-|x-stainless-|user-agent$)`: "",
	})
	require.NoError(t, err)
	channel := &model.Channel{
		Type: constant.ChannelTypeAdvancedCustom, Name: name, Key: key,
		BaseURL: &baseURL, Models: "gpt-5", Group: group, Status: common.ChannelStatusEnabled,
	}
	channel.SetSetting(dto.ChannelSettings{PassThroughBodyEnabled: true})
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{
			{IncomingPath: "/v1/messages", UpstreamPath: "/v1/messages", Converter: relayconvert.ConverterNone},
			{IncomingPath: "/v1/messages/count_tokens", UpstreamPath: "/v1/messages/count_tokens", Converter: relayconvert.ConverterNone},
			{IncomingPath: "/v1/chat/completions", UpstreamPath: "/v1/chat/completions", Converter: relayconvert.ConverterNone},
			{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses", Converter: relayconvert.ConverterNone},
		},
	}})
	headerJSON := string(headerOverride)
	channel.HeaderOverride = &headerJSON
	require.NoError(t, channel.Insert())
	return channel.Id
}
