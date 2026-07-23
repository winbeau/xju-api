package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	responsesWebsocketRequestCreate = "response.create"
	responsesWebsocketRequestAppend = "response.append"
	responsesWebsocketEventDone     = "response.done"
	responsesWebsocketEventFailed   = "response.failed"
	responsesWebsocketEventError    = "response.error"
	responsesWebsocketEventPartial  = "response.incomplete"
	responsesWebsocketMaxDuration   = 60 * time.Minute
)

var responsesWebsocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

type responsesWebsocketReadResult struct {
	messageType int
	payload     []byte
	err         error
}

type responsesWebsocketErrorEvent struct {
	Type   string            `json:"type"`
	Status int               `json:"status"`
	Error  types.OpenAIError `json:"error"`
}

type responsesWebsocketTurn struct {
	info       *relaycommon.RelayInfo
	usage      dto.Usage
	outputText strings.Builder
	generate   bool
}

type responsesWebsocketTerminal int

const (
	responsesWebsocketTerminalNone responsesWebsocketTerminal = iota
	responsesWebsocketTerminalCompleted
	responsesWebsocketTerminalError
)

// RelayResponsesWebsocket terminates the user's authenticated WebSocket at L1,
// pins one selected L2 channel for the connection, and relays sequential
// response.create turns while retaining normal new-api billing and usage logs.
func RelayResponsesWebsocket(c *gin.Context) {
	responseHeader := http.Header{}
	if turnState := strings.TrimSpace(c.GetHeader("X-Codex-Turn-State")); turnState != "" {
		responseHeader.Set("X-Codex-Turn-State", turnState)
	}
	clientConn, err := responsesWebsocketUpgrader.Upgrade(c.Writer, c.Request, responseHeader)
	if err != nil {
		return
	}
	defer clientConn.Close()

	done := make(chan struct{})
	defer close(done)
	clientReads := readResponsesWebsocket(clientConn, done)
	var upstreamConn *websocket.Conn
	var upstreamReads <-chan responsesWebsocketReadResult
	defer func() {
		if upstreamConn != nil {
			_ = upstreamConn.Close()
		}
	}()

	var selectedChannel *model.Channel
	var currentTurn *responsesWebsocketTurn
	pinnedModel := ""
	baseRequestID := c.GetString(common.RequestIdKey)
	if baseRequestID == "" {
		baseRequestID = common.NewRequestId()
	}
	turnIndex := 0
	connectionTimer := time.NewTimer(responsesWebsocketMaxDuration)
	defer connectionTimer.Stop()

	for {
		select {
		case result, ok := <-clientReads:
			if !ok || result.err != nil {
				refundResponsesWebsocketTurn(c, currentTurn)
				return
			}
			if result.messageType != websocket.TextMessage && result.messageType != websocket.BinaryMessage {
				continue
			}
			if currentTurn != nil {
				writeResponsesWebsocketError(c, clientConn, types.NewErrorWithStatusCode(
					errors.New("Responses WebSocket allows only one in-flight response per connection"),
					types.ErrorCodeInvalidRequest,
					http.StatusConflict,
					types.ErrOptionWithSkipRetry(),
				))
				continue
			}

			eventType, request, apiErr := parseResponsesWebsocketRequest(result.payload, pinnedModel)
			if apiErr != nil {
				writeResponsesWebsocketError(c, clientConn, apiErr)
				continue
			}
			if _, tokenErr := model.ValidateUserToken(common.GetContextKeyString(c, constant.ContextKeyTokenKey)); tokenErr != nil {
				writeResponsesWebsocketError(c, clientConn, types.NewErrorWithStatusCode(
					tokenErr,
					types.ErrorCodeAccessDenied,
					http.StatusUnauthorized,
					types.ErrOptionWithSkipRetry(),
				))
				return
			}

			if pinnedModel == "" {
				pinnedModel = request.Model
			}
			if selectedChannel == nil {
				selectedChannel, apiErr = middleware.SelectChannelForResponsesWebsocket(c, pinnedModel)
				if apiErr != nil {
					writeResponsesWebsocketError(c, clientConn, apiErr)
					return
				}
			}

			turnIndex++
			c.Set(common.RequestIdKey, fmt.Sprintf("%s-ws-%d", baseRequestID, turnIndex))
			currentTurn, result.payload, apiErr = prepareResponsesWebsocketTurn(c, eventType, request, result.payload)
			if apiErr != nil {
				writeResponsesWebsocketError(c, clientConn, apiErr)
				currentTurn = nil
				continue
			}

			if upstreamConn == nil {
				upstreamConn, apiErr = relay.DialResponsesWebsocket(c, currentTurn.info)
				if apiErr != nil {
					refundResponsesWebsocketTurn(c, currentTurn)
					currentTurn = nil
					processChannelError(c, *types.NewChannelError(
						selectedChannel.Id,
						selectedChannel.Type,
						selectedChannel.Name,
						selectedChannel.ChannelInfo.IsMultiKey,
						common.GetContextKeyString(c, constant.ContextKeyChannelKey),
						selectedChannel.GetAutoBan(),
					), apiErr)
					writeResponsesWebsocketError(c, clientConn, apiErr)
					return
				}
				upstreamReads = readResponsesWebsocket(upstreamConn, done)
			}

			if err = upstreamConn.WriteMessage(result.messageType, result.payload); err != nil {
				refundResponsesWebsocketTurn(c, currentTurn)
				currentTurn = nil
				writeResponsesWebsocketError(c, clientConn, types.NewError(
					fmt.Errorf("failed to send request to upstream websocket: %w", err),
					types.ErrorCodeDoRequestFailed,
					types.ErrOptionWithSkipRetry(),
				))
				return
			}

		case result, ok := <-upstreamReads:
			if !ok || result.err != nil {
				refundResponsesWebsocketTurn(c, currentTurn)
				if result.err != nil {
					writeResponsesWebsocketError(c, clientConn, types.NewError(
						fmt.Errorf("upstream websocket closed: %w", result.err),
						types.ErrorCodeBadResponse,
						types.ErrOptionWithSkipRetry(),
					))
				}
				return
			}

			terminal := responsesWebsocketTerminalNone
			if currentTurn != nil {
				terminal = currentTurn.observe(c, result.payload)
			}
			if err = clientConn.WriteMessage(result.messageType, result.payload); err != nil {
				refundResponsesWebsocketTurn(c, currentTurn)
				return
			}

			switch terminal {
			case responsesWebsocketTerminalCompleted:
				currentTurn.finish(c)
				service.RecordChannelAffinity(c, selectedChannel.Id)
				currentTurn = nil
			case responsesWebsocketTerminalError:
				refundResponsesWebsocketTurn(c, currentTurn)
				currentTurn = nil
			}

		case <-connectionTimer.C:
			refundResponsesWebsocketTurn(c, currentTurn)
			_ = clientConn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Responses WebSocket connection duration limit reached"),
				time.Now().Add(5*time.Second),
			)
			return
		}
	}
}

func readResponsesWebsocket(conn *websocket.Conn, done <-chan struct{}) <-chan responsesWebsocketReadResult {
	results := make(chan responsesWebsocketReadResult, 1)
	go func() {
		defer close(results)
		for {
			messageType, payload, err := conn.ReadMessage()
			result := responsesWebsocketReadResult{
				messageType: messageType,
				payload:     payload,
				err:         err,
			}
			select {
			case results <- result:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return results
}

func parseResponsesWebsocketRequest(payload []byte, pinnedModel string) (string, *dto.OpenAIResponsesRequest, *types.NewAPIError) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := common.Unmarshal(payload, &envelope); err != nil {
		return "", nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	eventType := strings.TrimSpace(envelope.Type)
	if eventType != responsesWebsocketRequestCreate && eventType != responsesWebsocketRequestAppend {
		return "", nil, types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported Responses WebSocket event type %q", eventType),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if eventType == responsesWebsocketRequestAppend && pinnedModel == "" {
		return "", nil, types.NewErrorWithStatusCode(
			errors.New("response.append cannot be the first event on a connection"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request := &dto.OpenAIResponsesRequest{}
	if err := common.Unmarshal(payload, request); err != nil {
		return "", nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	explicitModel := strings.TrimSpace(request.Model)
	if explicitModel == "" {
		request.Model = pinnedModel
	} else if pinnedModel != "" && explicitModel != pinnedModel {
		return "", nil, types.NewErrorWithStatusCode(
			fmt.Errorf("model cannot change within one Responses WebSocket connection: %s -> %s", pinnedModel, explicitModel),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if request.Input == nil && request.Generate != nil && !*request.Generate {
		request.Input = []byte("[]")
	}
	if err := relayhelper.ValidateResponsesRequest(request); err != nil {
		return "", nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return eventType, request, nil
}

func prepareResponsesWebsocketTurn(
	c *gin.Context,
	eventType string,
	request *dto.OpenAIResponsesRequest,
	rawPayload []byte,
) (*responsesWebsocketTurn, []byte, *types.NewAPIError) {
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	c.Set("image_generation_call", false)
	c.Set("image_generation_call_quality", "")
	c.Set("image_generation_call_size", "")
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIResponses, request, nil)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithSkipRetry())
	}
	info.IsStream = true
	common.SetContextKey(c, constant.ContextKeyIsStream, true)

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}
	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			return nil, nil, types.NewErrorWithStatusCode(
				errors.New("request contains sensitive words"),
				types.ErrorCodeSensitiveWordsDetected,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, info)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
	}
	info.SetEstimatePromptTokens(tokens)

	preparedPayload, apiErr := relay.PrepareResponsesWebsocketRequest(c, info, request, eventType, rawPayload)
	if apiErr != nil {
		return nil, nil, apiErr
	}
	priceData, err := relayhelper.ModelPriceHelper(c, info, tokens, meta)
	if err != nil {
		return nil, nil, types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if !priceData.FreeModel {
		if apiErr = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, info); apiErr != nil {
			return nil, nil, apiErr
		}
	}

	generate := request.Generate == nil || *request.Generate
	return &responsesWebsocketTurn{
		info:     info,
		generate: generate,
	}, preparedPayload, nil
}

func (turn *responsesWebsocketTurn) observe(c *gin.Context, payload []byte) responsesWebsocketTerminal {
	if turn == nil || turn.info == nil {
		return responsesWebsocketTerminalNone
	}
	turn.info.SetFirstResponseTime()

	var event dto.ResponsesStreamResponse
	if err := common.Unmarshal(payload, &event); err != nil {
		logger.LogDebug(c, "failed to parse Responses WebSocket event for usage: %s", err.Error())
		return responsesWebsocketTerminalNone
	}
	switch event.Type {
	case "response.output_text.delta":
		turn.outputText.WriteString(event.Delta)
	case dto.ResponsesOutputTypeItemDone:
		if event.Item != nil && event.Item.Type == dto.BuildInCallWebSearchCall &&
			turn.info.ResponsesUsageInfo != nil && turn.info.ResponsesUsageInfo.BuiltInTools != nil {
			tool := turn.info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]
			if tool == nil {
				tool = &relaycommon.BuildInToolInfo{ToolName: dto.BuildInToolWebSearchPreview}
				turn.info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview] = tool
			}
			tool.CallCount++
		}
	case "response.completed", responsesWebsocketEventDone, responsesWebsocketEventPartial:
		if event.Response != nil && event.Response.Usage != nil {
			upstreamUsage := event.Response.Usage
			turn.usage.PromptTokens = upstreamUsage.InputTokens
			if turn.usage.PromptTokens == 0 {
				turn.usage.PromptTokens = upstreamUsage.PromptTokens
			}
			turn.usage.CompletionTokens = upstreamUsage.OutputTokens
			if turn.usage.CompletionTokens == 0 {
				turn.usage.CompletionTokens = upstreamUsage.CompletionTokens
			}
			turn.usage.TotalTokens = upstreamUsage.TotalTokens
			if upstreamUsage.InputTokensDetails != nil {
				turn.usage.PromptTokensDetails = *upstreamUsage.InputTokensDetails
			}
		}
		if event.Response != nil && event.Response.HasImageGenerationCall() && c != nil {
			c.Set("image_generation_call", true)
			c.Set("image_generation_call_quality", event.Response.GetQuality())
			c.Set("image_generation_call_size", event.Response.GetSize())
		}
		return responsesWebsocketTerminalCompleted
	case "error", responsesWebsocketEventFailed, responsesWebsocketEventError:
		return responsesWebsocketTerminalError
	}
	return responsesWebsocketTerminalNone
}

func (turn *responsesWebsocketTurn) finish(c *gin.Context) {
	if turn == nil || turn.info == nil {
		return
	}
	if !turn.generate && turn.usage.TotalTokens == 0 {
		if err := service.SettleBilling(c, turn.info, 0); err != nil {
			logger.LogError(c, "failed to settle Responses WebSocket warmup billing: "+err.Error())
		}
		return
	}
	if turn.usage.CompletionTokens == 0 && turn.outputText.Len() > 0 {
		turn.usage.CompletionTokens = service.CountTextToken(turn.outputText.String(), turn.info.UpstreamModelName)
	}
	if turn.usage.PromptTokens == 0 && turn.usage.CompletionTokens != 0 {
		turn.usage.PromptTokens = turn.info.GetEstimatePromptTokens()
	}
	if turn.usage.TotalTokens == 0 {
		turn.usage.TotalTokens = turn.usage.PromptTokens + turn.usage.CompletionTokens
	}
	service.PostTextConsumeQuota(c, turn.info, &turn.usage, nil)
}

func refundResponsesWebsocketTurn(c *gin.Context, turn *responsesWebsocketTurn) {
	if turn != nil && turn.info != nil && turn.info.Billing != nil {
		turn.info.Billing.Refund(c)
	}
}

func writeResponsesWebsocketError(c *gin.Context, conn *websocket.Conn, apiErr *types.NewAPIError) {
	if conn == nil || apiErr == nil {
		return
	}
	status := apiErr.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	payload, err := common.Marshal(responsesWebsocketErrorEvent{
		Type:   "error",
		Status: status,
		Error:  apiErr.ToOpenAIError(),
	})
	if err != nil {
		logger.LogError(c, "failed to marshal Responses WebSocket error: "+err.Error())
		return
	}
	if err = conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		logger.LogDebug(c, "failed to write Responses WebSocket error: %s", err.Error())
	}
}
