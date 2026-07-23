package relay

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/sjson"
)

// PrepareResponsesWebsocketRequest applies the same model mapping, field
// filtering, request conversion, and parameter overrides as HTTP Responses
// relay, then restores the WebSocket event type required by the upstream.
func PrepareResponsesWebsocketRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request *dto.OpenAIResponsesRequest,
	eventType string,
	rawPayload []byte,
) ([]byte, *types.NewAPIError) {
	if info == nil || request == nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("invalid Responses WebSocket request"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	info.InitChannelMeta(c)
	requestCopy, err := common.DeepCopy(request)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err = helper.ModelMappedHelper(c, info, requestCopy); err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return append([]byte(nil), rawPayload...), nil
	}

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *requestCopy)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	payload, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	payload, err = relaycommon.RemoveDisabledFields(payload, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		payload, err = relaycommon.ApplyParamOverrideWithRelayInfo(payload, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}
	payload, err = sjson.SetBytes(payload, "type", eventType)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return payload, nil
}

// DialResponsesWebsocket opens the upstream WebSocket after channel selection.
// The selected channel must expose GET /v1/responses with an Upgrade handshake.
func DialResponsesWebsocket(c *gin.Context, info *relaycommon.RelayInfo) (*websocket.Conn, *types.NewAPIError) {
	if info == nil {
		return nil, types.NewError(fmt.Errorf("relay info is nil"), types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithSkipRetry())
	}
	if info.ChannelMeta == nil {
		info.InitChannelMeta(c)
	}
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	conn, err := channel.DoWssRequest(adaptor, c, info, nil)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return conn, nil
}
