// model_resolution_test.go contains unit tests for model resolution and candidate selection.
package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestBuildModelListResolver(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		input     string
		wantModel string
		wantFound bool
	}{
		{
			name:      "nil config returns not found",
			cfg:       nil,
			input:     "gpt-4o",
			wantModel: "",
			wantFound: false,
		},
		{
			name:      "empty input returns not found",
			cfg:       &config.Config{},
			input:     "",
			wantModel: "",
			wantFound: false,
		},
		{
			name:      "whitespace-only input returns not found",
			cfg:       &config.Config{},
			input:     "   ",
			wantModel: "",
			wantFound: false,
		},
		{
			name: "match by model_name via GetModelConfig",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "my-gpt", Model: "openai/gpt-4o"},
				},
			},
			input:     "my-gpt",
			wantModel: "openai/gpt-4o",
			wantFound: true,
		},
		{
			name: "match by full model string in model list",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "alias", Model: "anthropic/claude-sonnet-4.6"},
				},
			},
			input:     "anthropic/claude-sonnet-4.6",
			wantModel: "anthropic/claude-sonnet-4.6",
			wantFound: true,
		},
		{
			name: "match by model ID without provider prefix",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "alias", Model: "deepseek/deepseek-chat"},
				},
			},
			input:     "deepseek-chat",
			wantModel: "deepseek/deepseek-chat",
			wantFound: true,
		},
		{
			name: "model without provider prefix gets openai prepended",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "bare", Model: "gpt-4o-mini"},
				},
			},
			input:     "bare",
			wantModel: "openai/gpt-4o-mini",
			wantFound: true,
		},
		{
			name: "no match returns not found",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "existing", Model: "openai/gpt-4o"},
				},
			},
			input:     "nonexistent-model",
			wantModel: "",
			wantFound: false,
		},
		{
			name: "empty model in list is skipped",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "empty", Model: ""},
					{ModelName: "valid", Model: "openai/gpt-4o"},
				},
			},
			input:     "openai/gpt-4o",
			wantModel: "openai/gpt-4o",
			wantFound: true,
		},
		{
			name: "whitespace model in list is skipped",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "space", Model: "   "},
				},
			},
			input:     "space",
			wantModel: "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := buildModelListResolver(tt.cfg)
			gotModel, gotFound := resolver(tt.input)
			assert.Equal(t, tt.wantFound, gotFound, "found mismatch")
			assert.Equal(t, tt.wantModel, gotModel, "model mismatch")
		})
	}
}

func TestResolveModelCandidates(t *testing.T) {
	tests := []struct {
		name            string
		cfg             *config.Config
		defaultProvider string
		primary         string
		fallbacks       []string
		wantLen         int
		wantFirst       *providers.FallbackCandidate
	}{
		{
			name:            "primary model with provider prefix",
			cfg:             &config.Config{},
			defaultProvider: "openai",
			primary:         "anthropic/claude-sonnet-4.6",
			fallbacks:       nil,
			wantLen:         1,
			wantFirst:       &providers.FallbackCandidate{Provider: "anthropic", Model: "claude-sonnet-4.6"},
		},
		{
			name:            "primary model without prefix uses default provider",
			cfg:             &config.Config{},
			defaultProvider: "openai",
			primary:         "gpt-4o",
			fallbacks:       nil,
			wantLen:         1,
			wantFirst:       &providers.FallbackCandidate{Provider: "openai", Model: "gpt-4o"},
		},
		{
			name:            "primary with fallbacks",
			cfg:             &config.Config{},
			defaultProvider: "openai",
			primary:         "openai/gpt-4o",
			fallbacks:       []string{"anthropic/claude-sonnet-4.6"},
			wantLen:         2,
			wantFirst:       &providers.FallbackCandidate{Provider: "openai", Model: "gpt-4o"},
		},
		{
			name: "primary resolved via model list alias",
			cfg: &config.Config{
				ModelList: []*config.ModelConfig{
					{ModelName: "my-model", Model: "gemini/gemini-pro"},
				},
			},
			defaultProvider: "openai",
			primary:         "my-model",
			fallbacks:       nil,
			wantLen:         1,
			wantFirst:       &providers.FallbackCandidate{Provider: "gemini", Model: "gemini-pro"},
		},
		{
			name:            "empty primary produces no candidates",
			cfg:             &config.Config{},
			defaultProvider: "openai",
			primary:         "",
			fallbacks:       nil,
			wantLen:         0,
			wantFirst:       nil,
		},
		{
			name:            "nil config still works for direct model refs",
			cfg:             nil,
			defaultProvider: "openai",
			primary:         "openai/gpt-4o",
			fallbacks:       nil,
			wantLen:         1,
			wantFirst:       &providers.FallbackCandidate{Provider: "openai", Model: "gpt-4o"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := resolveModelCandidates(tt.cfg, tt.defaultProvider, tt.primary, tt.fallbacks)
			assert.Len(t, candidates, tt.wantLen)
			if tt.wantFirst != nil && len(candidates) > 0 {
				assert.Equal(t, tt.wantFirst.Provider, candidates[0].Provider)
				assert.Equal(t, tt.wantFirst.Model, candidates[0].Model)
			}
		})
	}
}

func TestResolvedCandidateModel(t *testing.T) {
	tests := []struct {
		name       string
		candidates []providers.FallbackCandidate
		fallback   string
		want       string
	}{
		{
			name:       "returns first candidate model",
			candidates: []providers.FallbackCandidate{{Provider: "openai", Model: "gpt-4o"}},
			fallback:   "default-model",
			want:       "gpt-4o",
		},
		{
			name:       "returns fallback when candidates empty",
			candidates: nil,
			fallback:   "default-model",
			want:       "default-model",
		},
		{
			name:       "returns fallback when candidates slice is empty",
			candidates: []providers.FallbackCandidate{},
			fallback:   "fallback-model",
			want:       "fallback-model",
		},
		{
			name:       "returns fallback when first candidate model is whitespace",
			candidates: []providers.FallbackCandidate{{Provider: "openai", Model: "  "}},
			fallback:   "fallback-model",
			want:       "fallback-model",
		},
		{
			name:       "returns fallback when first candidate model is empty",
			candidates: []providers.FallbackCandidate{{Provider: "openai", Model: ""}},
			fallback:   "fallback-model",
			want:       "fallback-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvedCandidateModel(tt.candidates, tt.fallback)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolvedCandidateProvider(t *testing.T) {
	tests := []struct {
		name       string
		candidates []providers.FallbackCandidate
		fallback   string
		want       string
	}{
		{
			name:       "returns first candidate provider",
			candidates: []providers.FallbackCandidate{{Provider: "anthropic", Model: "claude-sonnet-4.6"}},
			fallback:   "openai",
			want:       "anthropic",
		},
		{
			name:       "returns fallback when candidates empty",
			candidates: nil,
			fallback:   "openai",
			want:       "openai",
		},
		{
			name:       "returns fallback when candidates slice is empty",
			candidates: []providers.FallbackCandidate{},
			fallback:   "openai",
			want:       "openai",
		},
		{
			name:       "returns fallback when first candidate provider is whitespace",
			candidates: []providers.FallbackCandidate{{Provider: "  ", Model: "model"}},
			fallback:   "default-provider",
			want:       "default-provider",
		},
		{
			name:       "returns fallback when first candidate provider is empty",
			candidates: []providers.FallbackCandidate{{Provider: "", Model: "model"}},
			fallback:   "default-provider",
			want:       "default-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvedCandidateProvider(tt.candidates, tt.fallback)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolvedModelConfig(t *testing.T) {
	t.Run("nil config returns error", func(t *testing.T) {
		result, err := resolvedModelConfig(nil, "any-model", "/workspace")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "config is nil")
	})

	t.Run("model not found returns error", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{
				{ModelName: "existing", Model: "openai/gpt-4o"},
			},
		}
		result, err := resolvedModelConfig(cfg, "nonexistent", "/workspace")
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("found model config is cloned", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{
				{ModelName: "test-model", Model: "openai/gpt-4o", APIBase: "https://api.openai.com"},
			},
		}
		result, err := resolvedModelConfig(cfg, "test-model", "/workspace")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "openai/gpt-4o", result.Model)
		assert.Equal(t, "https://api.openai.com", result.APIBase)

		// Mutating the clone should not affect the original
		result.APIBase = "https://modified.com"
		original, _ := cfg.GetModelConfig("test-model")
		assert.Equal(t, "https://api.openai.com", original.APIBase)
	})

	t.Run("workspace is set from parameter when empty", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{
				{ModelName: "test-model", Model: "openai/gpt-4o"},
			},
		}
		result, err := resolvedModelConfig(cfg, "test-model", "/my/workspace")
		require.NoError(t, err)
		assert.Equal(t, "/my/workspace", result.Workspace)
	})

	t.Run("workspace is preserved when already set", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{
				{ModelName: "test-model", Model: "openai/gpt-4o", Workspace: "/original"},
			},
		}
		result, err := resolvedModelConfig(cfg, "test-model", "/different")
		require.NoError(t, err)
		assert.Equal(t, "/original", result.Workspace)
	})

	t.Run("model name with whitespace is trimmed", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{
				{ModelName: "test-model", Model: "openai/gpt-4o"},
			},
		}
		result, err := resolvedModelConfig(cfg, "  test-model  ", "/workspace")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "openai/gpt-4o", result.Model)
	})

	t.Run("empty model list returns error", func(t *testing.T) {
		cfg := &config.Config{
			ModelList: []*config.ModelConfig{},
		}
		result, err := resolvedModelConfig(cfg, "any", "/workspace")
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
