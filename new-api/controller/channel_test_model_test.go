package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string { return &s }

func TestResolveChannelTestModel(t *testing.T) {
	cases := []struct {
		name      string
		channel   *model.Channel
		testModel string
		want      string
	}{
		{"explicit passes through", &model.Channel{}, " gpt-image-2 ", "gpt-image-2"},
		{"empty falls back to TestModel", &model.Channel{TestModel: strPtr("gpt-5.5")}, "", "gpt-5.5"},
		{"empty + no TestModel uses first model", &model.Channel{Models: "gpt-5.5,gpt-image-2"}, "", "gpt-5.5"},
		{"empty + nothing uses default", &model.Channel{}, "", "gpt-4o-mini"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveChannelTestModel(tc.channel, tc.testModel))
		})
	}
}

// The latency routing hinges on this classification: image models must go to the
// image latency column, chat/codex models to the chat column.
func TestChannelTestLatencyRouting(t *testing.T) {
	assert.True(t, common.IsImageGenerationModel("gpt-image-2"), "gpt-image-2 is an image model")
	assert.True(t, common.IsImageGenerationModel("gpt-image-1.5"), "gpt-image-1.5 is an image model")
	assert.False(t, common.IsImageGenerationModel("gpt-5.5"), "gpt-5.5 is a chat model")
	assert.False(t, common.IsImageGenerationModel("codex-auto-review"), "codex model is a chat model")
}
