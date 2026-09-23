package cmd

import (
	"testing"

	"github.com/Geogboe/rog/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOptionalLLMConfigDoesNotWarn(t *testing.T) {
	assert.Empty(t, llmConfigWarnings(nil))
	assert.Empty(t, llmConfigWarnings(&config.LLMConfig{}))
	assert.Len(t, llmConfigWarnings(&config.LLMConfig{Endpoint: "http://localhost"}), 1)
}

func TestConfigForDisplayRedactsAPIKey(t *testing.T) {
	cfg := &config.Config{LLM: &config.LLMConfig{APIKey: "example-sensitive-value"}}
	shown := configForDisplay(cfg)
	data, err := yaml.Marshal(shown)
	require.NoError(t, err)
	assert.NotContains(t, string(data), cfg.LLM.APIKey)
	assert.Equal(t, "example-sensitive-value", cfg.LLM.APIKey)
}
