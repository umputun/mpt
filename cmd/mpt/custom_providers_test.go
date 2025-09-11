package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomSpecUnmarshalFlag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected customSpec
		wantErr  bool
		errMsg   string
	}{
		{
			name:  "full spec with all fields",
			input: "url=https://api.example.com,model=gpt-4,api-key=secret,temperature=0.5,max-tokens=8k,name=MyProvider",
			expected: customSpec{
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
			expected: customSpec{
				URL:         "http://localhost:8080",
				Model:       "local-llm",
				Temperature: 0.7,   // default
				MaxTokens:   16384, // default
				Enabled:     true,  // default
			},
		},
		{
			name:  "spec with URL aliases",
			input: "base-url=http://test.com,model=test",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with base_url alias",
			input: "base_url=http://test.com,model=test",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with baseurl alias",
			input: "baseurl=http://test.com,model=test",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with api-key aliases",
			input: "url=http://test.com,model=test,api_key=key1,temperature=0.3",
			expected: customSpec{
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
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				APIKey:      "key2",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with max-tokens aliases",
			input: "url=http://test.com,model=test,max_tokens=4k",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   4096,
				Enabled:     true,
			},
		},
		{
			name:  "spec with maxtokens alias",
			input: "url=http://test.com,model=test,maxtokens=2048",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   2048,
				Enabled:     true,
			},
		},
		{
			name:  "spec with temp alias",
			input: "url=http://test.com,model=test,temp=0.3",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.3,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "disabled provider",
			input: "url=http://test.com,model=test,enabled=false",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     false,
			},
		},
		{
			name:  "spec with spaces in values",
			input: "url=http://test.com,model=test model,name=My Provider",
			expected: customSpec{
				URL:         "http://test.com",
				Model:       "test model",
				Name:        "My Provider",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with URL containing port and path",
			input: "url=http://localhost:8080/v1/api,model=local",
			expected: customSpec{
				URL:         "http://localhost:8080/v1/api",
				Model:       "local",
				Temperature: 0.7,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "spec with https URL",
			input: "url=https://api.openai.com/v1,model=gpt-4",
			expected: customSpec{
				URL:         "https://api.openai.com/v1",
				Model:       "gpt-4",
				Temperature: 0.7,
				MaxTokens:   16384,
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
			input:   "url=test,model=test,max-tokens=invalid",
			wantErr: true,
			errMsg:  "invalid max-tokens 'invalid'",
		},
		{
			name:    "invalid enabled value",
			input:   "url=test,model=test,enabled=yes",
			wantErr: true,
			errMsg:  "invalid enabled value 'yes'",
		},
		{
			name:  "max tokens with k suffix",
			input: "url=test,model=test,max-tokens=32k",
			expected: customSpec{
				URL:         "test",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   32768,
				Enabled:     true,
			},
		},
		{
			name:  "max tokens with m suffix",
			input: "url=test,model=test,max-tokens=1m",
			expected: customSpec{
				URL:         "test",
				Model:       "test",
				Temperature: 0.7,
				MaxTokens:   1048576,
				Enabled:     true,
			},
		},
		{
			name:  "temperature at boundaries",
			input: "url=test,model=test,temperature=0",
			expected: customSpec{
				URL:         "test",
				Model:       "test",
				Temperature: 0,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
		{
			name:  "temperature at upper boundary",
			input: "url=test,model=test,temperature=2.0",
			expected: customSpec{
				URL:         "test",
				Model:       "test",
				Temperature: 2.0,
				MaxTokens:   16384,
				Enabled:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec customSpec
			err := spec.UnmarshalFlag(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, spec)
		})
	}
}

func TestValidateProviderID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid lowercase",
			id:      "openrouter",
			wantErr: false,
		},
		{
			name:    "valid with numbers",
			id:      "provider123",
			wantErr: false,
		},
		{
			name:    "valid with hyphen",
			id:      "open-router",
			wantErr: false,
		},
		{
			name:    "valid with underscore",
			id:      "open_router",
			wantErr: false,
		},
		{
			name:    "valid complex",
			id:      "test-provider_123",
			wantErr: false,
		},
		{
			name:    "invalid - empty",
			id:      "",
			wantErr: true,
			errMsg:  "provider ID cannot be empty",
		},
		{
			name:    "invalid - uppercase",
			id:      "OpenRouter",
			wantErr: true,
			errMsg:  "invalid character 'O'",
		},
		{
			name:    "invalid - space",
			id:      "open router",
			wantErr: true,
			errMsg:  "invalid character ' '",
		},
		{
			name:    "invalid - special char",
			id:      "open.router",
			wantErr: true,
			errMsg:  "invalid character '.'",
		},
		{
			name:    "invalid - exclamation",
			id:      "provider!",
			wantErr: true,
			errMsg:  "invalid character '!'",
		},
		{
			name:    "invalid - slash",
			id:      "provider/test",
			wantErr: true,
			errMsg:  "invalid character '/'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderID(tt.id)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestNormalizeProviderID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already lowercase",
			input:    "openrouter",
			expected: "openrouter",
		},
		{
			name:     "uppercase to lowercase",
			input:    "OpenRouter",
			expected: "openrouter",
		},
		{
			name:     "mixed case",
			input:    "OpEn_RoUtEr",
			expected: "open_router",
		},
		{
			name:     "with spaces to trim",
			input:    "  provider  ",
			expected: "provider",
		},
		{
			name:     "complex",
			input:    " Test-Provider_123 ",
			expected: "test-provider_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeProviderID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to find index of rune in string
func indexOf(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// Helper function to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// Helper to clear all CUSTOM_* env vars
func clearCustomEnv() {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CUSTOM_") {
			if idx := indexOf(env, '='); idx != -1 {
				os.Unsetenv(env[:idx])
			}
		}
	}
}

func TestParseCustomProvidersFromEnv(t *testing.T) {

	t.Run("parse multiple providers", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		// Set test environment
		os.Setenv("CUSTOM_OPENROUTER_URL", "https://openrouter.ai/api/v1")
		os.Setenv("CUSTOM_OPENROUTER_MODEL", "claude-3.5-sonnet")
		os.Setenv("CUSTOM_OPENROUTER_API_KEY", "secret-key")
		os.Setenv("CUSTOM_OPENROUTER_TEMPERATURE", "0.5")
		os.Setenv("CUSTOM_OPENROUTER_MAX_TOKENS", "8192")
		os.Setenv("CUSTOM_OPENROUTER_NAME", "OpenRouter Provider")

		os.Setenv("CUSTOM_LOCAL_URL", "http://localhost:11434")
		os.Setenv("CUSTOM_LOCAL_MODEL", "mixtral")
		os.Setenv("CUSTOM_LOCAL_TEMPERATURE", "0.3")
		os.Setenv("CUSTOM_LOCAL_ENABLED", "true")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 2)
		assert.Empty(t, warnings)

		// Check OpenRouter
		or := providers["openrouter"]
		assert.Equal(t, "https://openrouter.ai/api/v1", or.URL)
		assert.Equal(t, "claude-3.5-sonnet", or.Model)
		assert.Equal(t, "secret-key", or.APIKey)
		assert.InEpsilon(t, float32(0.5), or.Temperature, 0.001)
		assert.Equal(t, SizeValue(8192), or.MaxTokens)
		assert.Equal(t, "OpenRouter Provider", or.Name)
		assert.True(t, or.Enabled)

		// Check Local
		local := providers["local"]
		assert.Equal(t, "http://localhost:11434", local.URL)
		assert.Equal(t, "mixtral", local.Model)
		assert.InEpsilon(t, float32(0.3), local.Temperature, 0.001)
		assert.True(t, local.Enabled)
	})

	t.Run("skip legacy variables", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		// These should be ignored
		os.Setenv("CUSTOM_URL", "http://legacy.com")
		os.Setenv("CUSTOM_MODEL", "legacy-model")
		os.Setenv("CUSTOM_API_KEY", "legacy-key")
		os.Setenv("CUSTOM_MAX_TOKENS", "4096")
		os.Setenv("CUSTOM_TEMPERATURE", "0.9")
		os.Setenv("CUSTOM_ENABLED", "true")
		os.Setenv("CUSTOM_NAME", "Legacy Provider")

		// This should be parsed
		os.Setenv("CUSTOM_NEW_URL", "http://new.com")
		os.Setenv("CUSTOM_NEW_MODEL", "new-model")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 1)
		assert.Empty(t, warnings)

		// Only NEW should be parsed
		newProvider := providers["new"]
		assert.Equal(t, "http://new.com", newProvider.URL)
		assert.Equal(t, "new-model", newProvider.Model)
	})

	t.Run("handle aliases", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		os.Setenv("CUSTOM_TEST1_BASE_URL", "http://test1.com")
		os.Setenv("CUSTOM_TEST1_MODEL", "model1")
		os.Setenv("CUSTOM_TEST1_API_KEY", "key1")

		os.Setenv("CUSTOM_TEST2_URL", "http://test2.com")
		os.Setenv("CUSTOM_TEST2_MODEL", "model2")
		os.Setenv("CUSTOM_TEST2_APIKEY", "key2")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 2)
		assert.Empty(t, warnings)

		assert.Equal(t, "http://test1.com", providers["test1"].URL)
		assert.Equal(t, "key1", providers["test1"].APIKey)

		assert.Equal(t, "http://test2.com", providers["test2"].URL)
		assert.Equal(t, "key2", providers["test2"].APIKey)
	})

	t.Run("invalid values generate warnings", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		os.Setenv("CUSTOM_BAD_URL", "http://bad.com")
		os.Setenv("CUSTOM_BAD_MODEL", "bad-model")
		os.Setenv("CUSTOM_BAD_TEMPERATURE", "invalid")
		os.Setenv("CUSTOM_BAD_MAX_TOKENS", "not-a-number")
		os.Setenv("CUSTOM_BAD_ENABLED", "maybe")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 1)
		assert.Len(t, warnings, 3) // temperature, max_tokens, enabled warnings

		// Check that warnings contain expected messages
		warningsStr := joinStrings(warnings, " ")
		assert.Contains(t, warningsStr, "invalid temperature")
		assert.Contains(t, warningsStr, "invalid max_tokens")
		assert.Contains(t, warningsStr, "invalid enabled")

		// Provider should still be created with defaults for invalid fields
		bad := providers["bad"]
		assert.Equal(t, "http://bad.com", bad.URL)
		assert.Equal(t, "bad-model", bad.Model)
		assert.InEpsilon(t, float32(0.7), bad.Temperature, 0.001) // default
		assert.Equal(t, SizeValue(16384), bad.MaxTokens)          // default
		assert.True(t, bad.Enabled)                               // default
	})

	t.Run("invalid provider ID generates warning", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		os.Setenv("CUSTOM_INVALID!_URL", "http://test.com")
		os.Setenv("CUSTOM_INVALID!_MODEL", "test")

		providers, warnings := parseCustomProvidersFromEnv()

		assert.Empty(t, providers)
		assert.Len(t, warnings, 2) // Two env vars with invalid ID
		assert.Contains(t, warnings[0], "invalid character '!'")
	})

	t.Run("temperature out of range", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		os.Setenv("CUSTOM_TEMP_URL", "http://temp.com")
		os.Setenv("CUSTOM_TEMP_MODEL", "temp-model")
		os.Setenv("CUSTOM_TEMP_TEMPERATURE", "5.0")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 1)
		assert.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "temperature 5")
		assert.Contains(t, warnings[0], "out of range")

		// Should use default temperature
		temp := providers["temp"]
		assert.InEpsilon(t, float32(0.7), temp.Temperature, 0.001)
	})

	t.Run("disabled provider", func(t *testing.T) {
		clearCustomEnv() // Clear before test
		os.Setenv("CUSTOM_DISABLED_URL", "http://disabled.com")
		os.Setenv("CUSTOM_DISABLED_MODEL", "disabled-model")
		os.Setenv("CUSTOM_DISABLED_ENABLED", "false")

		providers, warnings := parseCustomProvidersFromEnv()

		require.Len(t, providers, 1)
		assert.Empty(t, warnings)

		disabled := providers["disabled"]
		assert.False(t, disabled.Enabled)
	})
}

func TestInitializeCustomProviders(t *testing.T) {
	t.Run("precedence: CLI overrides environment", func(t *testing.T) {
		clearCustomEnv()
		// set environment variable
		os.Setenv("CUSTOM_TESTPROV_URL", "http://env.example.com")
		os.Setenv("CUSTOM_TESTPROV_MODEL", "env-model")
		os.Setenv("CUSTOM_TESTPROV_API_KEY", "env-key")

		opts := &options{
			Customs: map[string]customSpec{
				"testprov": {
					URL:     "http://cli.example.com",
					Model:   "cli-model",
					APIKey:  "cli-key",
					Enabled: true,
				},
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		require.Len(t, providers, 1)

		// verify CLI values were used
		p := providers[0]
		assert.Contains(t, p.Name(), "testprov")
	})

	t.Run("precedence: legacy overrides environment", func(t *testing.T) {
		clearCustomEnv()
		// set environment variable
		os.Setenv("CUSTOM_CUSTOM_URL", "http://env.example.com")
		os.Setenv("CUSTOM_CUSTOM_MODEL", "env-model")

		opts := &options{
			Custom: customOpenAIProvider{
				URL:     "http://legacy.example.com",
				Model:   "legacy-model",
				Enabled: true,
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		require.Len(t, providers, 1)

		// verify legacy values were used
		p := providers[0]
		assert.Contains(t, p.Name(), "custom")
	})

	t.Run("precedence: CLI overrides legacy", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Custom: customOpenAIProvider{
				URL:     "http://legacy.example.com",
				Model:   "legacy-model",
				Enabled: true,
			},
			Customs: map[string]customSpec{
				"custom": {
					URL:     "http://cli.example.com",
					Model:   "cli-model",
					Enabled: true,
				},
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		require.Len(t, providers, 1)

		// verify CLI values were used
		p := providers[0]
		assert.Contains(t, p.Name(), "custom")
	})

	t.Run("multiple providers from different sources", func(t *testing.T) {
		clearCustomEnv()
		// environment provider
		os.Setenv("CUSTOM_ENVPROV_URL", "http://env.example.com")
		os.Setenv("CUSTOM_ENVPROV_MODEL", "env-model")

		opts := &options{
			// legacy provider
			Custom: customOpenAIProvider{
				Name:    "legacy",
				URL:     "http://legacy.example.com",
				Model:   "legacy-model",
				Enabled: true,
			},
			// CLI provider
			Customs: map[string]customSpec{
				"cliprov": {
					URL:     "http://cli.example.com",
					Model:   "cli-model",
					Enabled: true,
				},
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		assert.Len(t, providers, 3)

		// verify all three providers are present
		names := make([]string, len(providers))
		for i, p := range providers {
			names[i] = p.Name()
		}
		assert.Contains(t, names, "cliprov")
		assert.Contains(t, names, "envprov")
		assert.Contains(t, names, "legacy")
	})

	t.Run("disabled providers are skipped", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Customs: map[string]customSpec{
				"enabled": {
					URL:     "http://enabled.example.com",
					Model:   "model",
					Enabled: true,
				},
				"disabled": {
					URL:     "http://disabled.example.com",
					Model:   "model",
					Enabled: false,
				},
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		require.Len(t, providers, 1)
		assert.Contains(t, providers[0].Name(), "enabled")
	})

	t.Run("validation errors collected", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Customs: map[string]customSpec{
				"nourl": {
					Model:   "model",
					Enabled: true,
				},
				"nomodel": {
					URL:     "http://example.com",
					Enabled: true,
				},
				"invalid-id!": {
					URL:     "http://example.com",
					Model:   "model",
					Enabled: true,
				},
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, providers)
		assert.Len(t, errors, 3)
		// check for specific error messages
		errStr := joinStrings(errors, " ")
		assert.Contains(t, errStr, "missing URL")
		assert.Contains(t, errStr, "missing model")
		assert.Contains(t, errStr, "invalid character")
	})

	t.Run("legacy custom with custom name", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Custom: customOpenAIProvider{
				Name:    "MyProvider",
				URL:     "http://example.com",
				Model:   "model",
				Enabled: true,
			},
		}

		providers, errors := initializeCustomProviders(opts)

		assert.Empty(t, errors)
		require.Len(t, providers, 1)
		assert.Equal(t, "MyProvider", providers[0].Name())
	})
}

func TestCollectCustomSecrets(t *testing.T) {
	t.Run("collects from all sources", func(t *testing.T) {
		clearCustomEnv()
		os.Setenv("CUSTOM_ENV_API_KEY", "env-secret")

		opts := &options{
			Custom: customOpenAIProvider{
				APIKey: "legacy-secret",
			},
			Customs: map[string]customSpec{
				"cli": {
					APIKey: "cli-secret",
				},
			},
		}

		secrets := collectCustomSecrets(opts)

		assert.Len(t, secrets, 3)
		assert.Contains(t, secrets, "env-secret")
		assert.Contains(t, secrets, "legacy-secret")
		assert.Contains(t, secrets, "cli-secret")
	})

	t.Run("no duplicates", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Customs: map[string]customSpec{
				"prov1": {
					APIKey: "same-secret",
				},
				"prov2": {
					APIKey: "same-secret",
				},
			},
		}

		secrets := collectCustomSecrets(opts)

		assert.Len(t, secrets, 1)
		assert.Contains(t, secrets, "same-secret")
	})

	t.Run("precedence in secret collection", func(t *testing.T) {
		clearCustomEnv()
		os.Setenv("CUSTOM_PROV_API_KEY", "env-secret")

		opts := &options{
			Customs: map[string]customSpec{
				"prov": {
					APIKey: "cli-secret",
				},
			},
		}

		secrets := collectCustomSecrets(opts)

		// should have CLI secret (higher precedence)
		assert.Len(t, secrets, 1)
		assert.Contains(t, secrets, "cli-secret")
	})
}

func TestAnyCustomProvidersEnabled(t *testing.T) {
	t.Run("legacy custom enabled", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Custom: customOpenAIProvider{
				Enabled: true,
			},
		}

		assert.True(t, anyCustomProvidersEnabled(opts))
	})

	t.Run("customs map has providers", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{
			Customs: map[string]customSpec{
				"test": {},
			},
		}

		assert.True(t, anyCustomProvidersEnabled(opts))
	})

	t.Run("environment has providers", func(t *testing.T) {
		clearCustomEnv()
		os.Setenv("CUSTOM_TEST_URL", "http://example.com")

		opts := &options{}

		assert.True(t, anyCustomProvidersEnabled(opts))
	})

	t.Run("no custom providers", func(t *testing.T) {
		clearCustomEnv()
		opts := &options{}

		assert.False(t, anyCustomProvidersEnabled(opts))
	})
}
