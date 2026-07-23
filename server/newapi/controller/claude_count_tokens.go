package controller

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const maxClaudeCountTokensResponseBytes = 16 << 20

// RelayClaudeCountTokens is an authenticated, group-aware pass-through for the
// Anthropic token counting endpoint. It deliberately skips the generation
// pricing/pre-consume/settlement pipeline while retaining TokenAuth,
// Distribute, private-pool isolation and Advanced Custom header replacement.
func RelayClaudeCountTokens(c *gin.Context) {
	request, err := helper.GetAndValidateClaudeRequest(c)
	if err != nil {
		respondClaudeCountTokensError(c, http.StatusBadRequest, err)
		return
	}

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatClaude, request, nil)
	if err != nil {
		respondClaudeCountTokensError(c, http.StatusInternalServerError, err)
		return
	}
	info.InitChannelMeta(c)
	if info.ChannelType != constant.ChannelTypeAdvancedCustom {
		respondClaudeCountTokensError(c, http.StatusServiceUnavailable, errors.New("selected channel does not support Anthropic token counting"))
		return
	}

	adaptor := relay.GetAdaptor(info.ApiType)
	if adaptor == nil {
		respondClaudeCountTokensError(c, http.StatusInternalServerError, errors.New("selected channel adaptor is unavailable"))
		return
	}
	adaptor.Init(info)

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		status := http.StatusBadRequest
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		respondClaudeCountTokensError(c, status, err)
		return
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		respondClaudeCountTokensError(c, http.StatusBadRequest, err)
		return
	}
	c.Request.Body = io.NopCloser(storage)
	info.UpstreamRequestBodySize = storage.Size()

	responseValue, err := adaptor.DoRequest(c, info, common.ReaderOnly(storage))
	if err != nil {
		respondClaudeCountTokensError(c, http.StatusBadGateway, err)
		return
	}
	response, ok := responseValue.(*http.Response)
	if !ok || response == nil {
		respondClaudeCountTokensError(c, http.StatusBadGateway, errors.New("invalid token counting response"))
		return
	}
	defer service.CloseResponseBodyGracefully(response)

	body, err := io.ReadAll(io.LimitReader(response.Body, maxClaudeCountTokensResponseBytes+1))
	if err != nil {
		respondClaudeCountTokensError(c, http.StatusBadGateway, err)
		return
	}
	if len(body) > maxClaudeCountTokensResponseBytes {
		respondClaudeCountTokensError(c, http.StatusBadGateway, errors.New("token counting response is too large"))
		return
	}
	service.IOCopyBytesGracefully(c, response, body)
}

func respondClaudeCountTokensError(c *gin.Context, status int, err error) {
	message := "token counting request failed"
	if err != nil && err.Error() != "" {
		message = err.Error()
	}
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "invalid_request_error",
			"message": message,
		},
	})
}
