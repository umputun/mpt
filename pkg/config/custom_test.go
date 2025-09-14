package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCustomSpec(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CustomSpec
		wantErr  bool
		errMsg   string
	}{
		{
			name:  "full spec with all fields",
			input: "url=https://api.example.com,model=gpt-4,api-key=secret,temperature=0.5,max-tokens=8k,name=MyProvider",
			expected: CustomSpec{
				URL:         "https://api.example.com",
				Model:       "gpt-4",
				APIKey:      "secret",
				Temperature: 0.5,
				MaxTokens:   8192,
				Name:        "MyProvider",
				Enabled:     true,
			},
		},
		{
			name:  "minimal spec with required fields only",
			input: "url=http://localhost:8080,model=local-llm",
			expected: CustomSpec{
				URL:         "http://localhost:8080",
				Model:       "local-llm",
				Temperature: -1,                     // unset, will use provider default
				MaxTokens:   defaultCustomMaxTokens, // default
				Enabled:     true,                   // default
			},
		},
		{
			name:  "spec with URL aliases",
			input: "base-url=http://test.com,model=test",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with base_url alias",
			input: "base_url=http://test.com,model=test",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with baseurl alias",
			input: "baseurl=http://test.com,model=test",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with api-key aliases",
			input: "url=http://test.com,model=test,api_key=key1,temperature=0.3",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				APIKey:      "key1",
				Temperature: 0.3,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with apikey alias",
			input: "url=http://test.com,model=test,apikey=key2",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				APIKey:      "key2",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with max-tokens aliases",
			input: "url=http://test.com,model=test,max_tokens=4k",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   4096,
				Enabled:     true,
			},
		},
		{
			name:  "spec with maxtokens alias",
			input: "url=http://test.com,model=test,maxtokens=2048",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   2048,
				Enabled:     true,
			},
		},
		{
			name:  "spec with temp alias",
			input: "url=http://test.com,model=test,temp=0.3",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.3,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with temperature=0 for deterministic output",
			input: "url=http://test.com,model=test,temperature=0",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0, // explicit 0 for deterministic output
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "disabled provider",
			input: "url=http://test.com,model=test,enabled=false",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     false,
			},
		},
		{
			name:  "spec with spaces in values",
			input: "url=http://test.com,model=test model,name=My Provider",
			expected: CustomSpec{
				URL:         "http://test.com",
				Model:       "test model",
				Name:        "My Provider",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with URL containing port and path",
			input: "url=http://localhost:8080/v1/api,model=local",
			expected: CustomSpec{
				URL:         "http://localhost:8080/v1/api",
				Model:       "local",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:  "spec with https URL",
			input: "url=https://api.openai.com/v1,model=gpt-4",
			expected: CustomSpec{
				URL:         "https://api.openai.com/v1",
				Model:       "gpt-4",
				Temperature: -1, // unset
				MaxTokens:   defaultCustomMaxTokens,
				Enabled:     true,
			},
		},
		{
			name:    "invalid format - missing equals",
			input:   "url=test,invalid",
			wantErr: true,
			errMsg:  "invalid format in 'invalid' (expected key=value)",
		},
		{
			name:    "invalid temperature - not a number",
			input:   "url=test,model=test,temperature=abc",
			wantErr: true,
			errMsg:  "invalid temperature 'abc'",
		},
		{
			name:    "temperature out of range - too high",
			input:   "url=test,model=test,temperature=3.0",
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2, got 3",
		},
		{
			name:    "temperature out of range - negative",
			input:   "url=test,model=test,temperature=-0.5",
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2, got -0.5",
		},
		{
			name:    "invalid max-tokens",
			input:   "url=test,model=test,max-tokens=abc",
			wantErr: true,
			errMsg:  "invalid max-tokens 'abc'",
		},
		{
			name:    "invalid enabled value",
			input:   "url=test,model=test,enabled=yes",
			wantErr: true,
			errMsg:  "invalid enabled value 'yes'",
		},
		{
			name:  "spec with human-readable max-tokens",
			input: "url=test,model=test,max-tokens=32k",
			expected: CustomSpec{
				URL:         "test",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   32768,
				Enabled:     true,
			},
		},
		{
			name:  "spec with megabyte max-tokens",
			input: "url=test,model=test,max-tokens=1m",
			expected: CustomSpec{
				URL:         "test",
				Model:       "test",
				Temperature: -1, // unset
				MaxTokens:   1048576,
				Enabled:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseCustomSpec(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, spec)
			}
		})
	}
}

func TestCustomProviderManager_parseCustomProvidersFromEnv(t *testing.T) {
	// helper to clear custom env vars
	clearCustomEnv := func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "CUSTOM_") {
				key := strings.Split(env, "=")[0]
				os.Unsetenv(key)
			}
		}
	}

	t.Run("parse multiple custom providers from env", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		// set up environment
		os.Setenv("CUSTOM_OPENROUTER_URL", "https://openrouter.ai/api/v1")
		os.Setenv("CUSTOM_OPENROUTER_MODEL", "claude-3.5-sonnet")
		os.Setenv("CUSTOM_OPENROUTER_API_KEY", "or-key-123")
		os.Setenv("CUSTOM_OPENROUTER_NAME", "OpenRouter")
		os.Setenv("CUSTOM_LOCAL_URL", "http://localhost:11434")
		os.Setenv("CUSTOM_LOCAL_MODEL", "mixtral:8x7b")
		os.Setenv("CUSTOM_LOCAL_TEMPERATURE", "0.5")

		manager := NewCustomProviderManager(nil, nil)
		providers, warnings := manager.parseCustomProvidersFromEnv()

		assert.Empty(t, warnings)
		assert.Len(t, providers, 2)

		// check openrouter provider
		or, exists := providers["openrouter"]
		assert.True(t, exists)
		assert.Equal(t, "https://openrouter.ai/api/v1", or.URL)
		assert.Equal(t, "claude-3.5-sonnet", or.Model)
		assert.Equal(t, "or-key-123", or.APIKey)
		assert.Equal(t, "OpenRouter", or.Name)

		// check local provider
		local, exists := providers["local"]
		assert.True(t, exists)
		assert.Equal(t, "http://localhost:11434", local.URL)
		assert.Equal(t, "mixtral:8x7b", local.Model)
		assert.InEpsilon(t, float32(0.5), local.Temperature, 0.0001)
	})

	t.Run("skip legacy env vars", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		// set legacy vars (should be ignored)
		os.Setenv("CUSTOM_URL", "http://legacy.com")
		os.Setenv("CUSTOM_MODEL", "legacy-model")
		os.Setenv("CUSTOM_API_KEY", "legacy-key")

		// set new-style var
		os.Setenv("CUSTOM_NEW_URL", "http://new.com")
		os.Setenv("CUSTOM_NEW_MODEL", "new-model")

		manager := NewCustomProviderManager(nil, nil)
		providers, warnings := manager.parseCustomProvidersFromEnv()

		assert.Empty(t, warnings)
		assert.Len(t, providers, 1)
		assert.Contains(t, providers, "new")
		assert.NotContains(t, providers, "custom")
	})

	t.Run("handle invalid provider ID", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		// invalid ID with special characters
		os.Setenv("CUSTOM_MY@PROVIDER_URL", "http://test.com")
		os.Setenv("CUSTOM_MY@PROVIDER_MODEL", "test")

		manager := NewCustomProviderManager(nil, nil)
		providers, warnings := manager.parseCustomProvidersFromEnv()

		assert.Len(t, warnings, 2) // two env vars skipped
		assert.Contains(t, warnings[0], "skipping env var")
		assert.Contains(t, warnings[0], "invalid character")
		assert.Empty(t, providers)
	})

	t.Run("handle invalid temperature", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		os.Setenv("CUSTOM_TEST_URL", "http://test.com")
		os.Setenv("CUSTOM_TEST_MODEL", "test")
		os.Setenv("CUSTOM_TEST_TEMPERATURE", "invalid")

		manager := NewCustomProviderManager(nil, nil)
		providers, warnings := manager.parseCustomProvidersFromEnv()

		assert.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "invalid temperature")
		assert.Len(t, providers, 1)
		assert.InEpsilon(t, float32(-1), providers["test"].Temperature, 0.0001) // keeps default
	})

	t.Run("disabled provider from env", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		os.Setenv("CUSTOM_TEST_URL", "http://test.com")
		os.Setenv("CUSTOM_TEST_MODEL", "test")
		os.Setenv("CUSTOM_TEST_ENABLED", "false")

		manager := NewCustomProviderManager(nil, nil)
		providers, warnings := manager.parseCustomProvidersFromEnv()

		assert.Empty(t, warnings)
		assert.Len(t, providers, 1)
		assert.False(t, providers["test"].Enabled)
	})
}

func TestCustomProviderManager_InitializeProviders(t *testing.T) {
	// helper to clear custom env vars
	clearCustomEnv := func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "CUSTOM_") {
				key := strings.Split(env, "=")[0]
				os.Unsetenv(key)
			}
		}
	}

	t.Run("initialize with valid providers", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test1": {
				URL:     "http://test1.com",
				Model:   "model1",
				Enabled: true,
			},
			"test2": {
				URL:     "http://test2.com",
				Model:   "model2",
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		providers, errors := manager.InitializeProviders()

		assert.Empty(t, errors)
		assert.Len(t, providers, 2)
	})

	t.Run("skip disabled providers", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"enabled": {
				URL:     "http://enabled.com",
				Model:   "model1",
				Enabled: true,
			},
			"disabled": {
				URL:     "http://disabled.com",
				Model:   "model2",
				Enabled: false,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		providers, errors := manager.InitializeProviders()

		assert.Empty(t, errors)
		assert.Len(t, providers, 1)
		assert.Equal(t, "enabled", providers[0].Name())
	})

	t.Run("error on missing URL", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test": {
				Model:   "model",
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		providers, errors := manager.InitializeProviders()

		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0], "missing URL")
		assert.Empty(t, providers)
	})

	t.Run("error on missing model", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test": {
				URL:     "http://test.com",
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		providers, errors := manager.InitializeProviders()

		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0], "missing model")
		assert.Empty(t, providers)
	})

	t.Run("precedence order - CLI overrides env", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		// set env provider
		os.Setenv("CUSTOM_TEST_URL", "http://env.com")
		os.Setenv("CUSTOM_TEST_MODEL", "env-model")

		// CLI provider with same ID
		customs := map[string]CustomSpec{
			"test": {
				URL:     "http://cli.com",
				Model:   "cli-model",
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		providers, errors := manager.InitializeProviders()

		assert.Empty(t, errors)
		assert.Len(t, providers, 1)
		// should use CLI values, not env
		assert.Equal(t, "test", providers[0].Name())
	})

	t.Run("legacy custom provider", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		legacy := &CustomSpec{
			Name:    "LegacyProvider",
			URL:     "http://legacy.com",
			Model:   "legacy-model",
			Enabled: true,
		}

		manager := NewCustomProviderManager(nil, legacy)
		providers, errors := manager.InitializeProviders()

		assert.Empty(t, errors)
		assert.Len(t, providers, 1)
		assert.Equal(t, "LegacyProvider", providers[0].Name())
	})
}

func TestCustomProviderManager_CollectSecrets(t *testing.T) {
	// helper to clear custom env vars
	clearCustomEnv := func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "CUSTOM_") {
				key := strings.Split(env, "=")[0]
				os.Unsetenv(key)
			}
		}
	}

	t.Run("collect unique secrets", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test1": {
				APIKey: "key1",
			},
			"test2": {
				APIKey: "key2",
			},
			"test3": {
				APIKey: "key1", // duplicate
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		secrets := manager.CollectSecrets()

		assert.Len(t, secrets, 2) // only unique
		assert.Contains(t, secrets, "key1")
		assert.Contains(t, secrets, "key2")
	})

	t.Run("include legacy secrets", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		legacy := &CustomSpec{
			APIKey:  "legacy-key",
			URL:     "http://legacy.com", // needs URL or Enabled to be considered
			Enabled: true,
		}

		customs := map[string]CustomSpec{
			"test": {
				APIKey: "test-key",
			},
		}

		manager := NewCustomProviderManager(customs, legacy)
		secrets := manager.CollectSecrets()

		assert.Len(t, secrets, 2)
		assert.Contains(t, secrets, "legacy-key")
		assert.Contains(t, secrets, "test-key")
	})
}

func TestCustomProviderManager_AnyEnabled(t *testing.T) {
	// helper to clear custom env vars
	clearCustomEnv := func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "CUSTOM_") {
				key := strings.Split(env, "=")[0]
				os.Unsetenv(key)
			}
		}
	}

	t.Run("no providers enabled", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		manager := NewCustomProviderManager(nil, nil)
		assert.False(t, manager.AnyEnabled())
	})

	t.Run("CLI customs enabled", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test": {
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		assert.True(t, manager.AnyEnabled())
	})

	t.Run("CLI customs configured but all disabled", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"test1": {
				URL:     "http://test1.com",
				Model:   "model1",
				Enabled: false,
			},
			"test2": {
				URL:     "http://test2.com",
				Model:   "model2",
				Enabled: false,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		assert.False(t, manager.AnyEnabled())
	})

	t.Run("mixed enabled and disabled customs", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		customs := map[string]CustomSpec{
			"disabled": {
				URL:     "http://disabled.com",
				Model:   "model1",
				Enabled: false,
			},
			"enabled": {
				URL:     "http://enabled.com",
				Model:   "model2",
				Enabled: true,
			},
		}

		manager := NewCustomProviderManager(customs, nil)
		assert.True(t, manager.AnyEnabled())
	})

	t.Run("legacy enabled", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		legacy := &CustomSpec{
			Enabled: true,
		}

		manager := NewCustomProviderManager(nil, legacy)
		assert.True(t, manager.AnyEnabled())
	})

	t.Run("env providers enabled", func(t *testing.T) {
		clearCustomEnv()
		defer clearCustomEnv()

		os.Setenv("CUSTOM_TEST_URL", "http://test.com")
		os.Setenv("CUSTOM_TEST_MODEL", "test")

		manager := NewCustomProviderManager(nil, nil)
		assert.True(t, manager.AnyEnabled())
	})
}

func TestParseSizeValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		// basic numbers
		{name: "simple number", input: "100", want: 100},
		{name: "zero", input: "0", want: 0},

		// with suffixes
		{name: "kilobytes", input: "1k", want: 1024},
		{name: "kilobytes uppercase", input: "1K", want: 1024},
		{name: "megabytes", input: "2m", want: 2 * 1024 * 1024},
		{name: "megabytes uppercase", input: "2M", want: 2 * 1024 * 1024},
		{name: "gigabytes", input: "1g", want: 1024 * 1024 * 1024},
		{name: "gigabytes uppercase", input: "1G", want: 1024 * 1024 * 1024},

		// edge cases
		{name: "empty string", input: "", wantErr: true},
		{name: "negative number", input: "-100", wantErr: true},
		{name: "invalid format", input: "abc", wantErr: true},
		{name: "invalid suffix", input: "100x", wantErr: true},
		{name: "whitespace", input: "  100  ", want: 100},
		{name: "whitespace with suffix", input: "  2k  ", want: 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// test through ParseCustomSpec since parseSizeValue is unexported
			spec, err := ParseCustomSpec("url=http://test.com,model=test,max-tokens=" + tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, spec.MaxTokens)
			}
		})
	}
}
