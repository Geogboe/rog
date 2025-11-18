package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Roots)
	assert.NotEmpty(t, cfg.Editor)
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "home dir expansion",
			input:   "~/test",
			wantErr: false,
		},
		{
			name:    "absolute path",
			input:   "/usr/local/bin",
			wantErr: false,
		},
		{
			name:    "relative path",
			input:   "relative/path",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				// Should be absolute
				assert.True(t, filepath.IsAbs(result))
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp dir for config
	tmpDir := t.TempDir()
	os.Setenv("ROG_CONFIG", filepath.Join(tmpDir, "config.yml"))
	defer os.Unsetenv("ROG_CONFIG")

	// Create config
	cfg := &Config{
		Roots: []Root{
			{
				Name:     "test",
				Path:     "/tmp/test",
				MaxDepth: 5,
				Exclude:  []string{"node_modules"},
			},
		},
		Editor: "vim",
	}

	// Save
	err := Save(cfg)
	require.NoError(t, err)

	// Load
	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, cfg.Roots[0].Name, loaded.Roots[0].Name)
	assert.Equal(t, cfg.Editor, loaded.Editor)
}

func TestEnvOverrides(t *testing.T) {
	// Create temp dir for config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")
	os.Setenv("ROG_CONFIG", configPath)
	defer os.Unsetenv("ROG_CONFIG")

	// Save base config
	cfg := &Config{
		Roots: []Root{
			{Name: "test", Path: "/tmp"},
		},
		Editor: "vi",
		LLM: &LLMConfig{
			Endpoint: "http://localhost:8080",
		},
	}
	require.NoError(t, Save(cfg))

	// Set env overrides
	os.Setenv("ROG_EDITOR", "code")
	os.Setenv("ROG_LLM_ENDPOINT", "http://localhost:11434/v1")
	os.Setenv("ROG_LLM_MODEL", "codellama")
	defer func() {
		os.Unsetenv("ROG_EDITOR")
		os.Unsetenv("ROG_LLM_ENDPOINT")
		os.Unsetenv("ROG_LLM_MODEL")
	}()

	// Load and check overrides
	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "code", loaded.Editor)
	assert.Equal(t, "http://localhost:11434/v1", loaded.LLM.Endpoint)
	assert.Equal(t, "codellama", loaded.LLM.Model)
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("ROG_CONFIG", filepath.Join(tmpDir, "nonexistent.yml"))
	defer os.Unsetenv("ROG_CONFIG")

	cfg, err := Load()
	require.NoError(t, err) // Should return default config
	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Roots)
}
