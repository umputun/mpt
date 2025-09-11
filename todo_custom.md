# TODO: Multiple Custom Providers Implementation

## Overview
Implement support for multiple custom OpenAI-compatible providers using go-flags map with UnmarshalFlag, enabling clean CLI syntax and environment variable configuration while maintaining full backward compatibility.

## Motivation
- Current implementation only supports a single custom provider
- Users need to work with multiple custom providers (OpenRouter, local Ollama, Perplexity, etc.)
- Previous attempt with `map[string]CustomOpenAIProvider` was reverted due to go-flags limitations with complex structs

## Solution Design

### Core Approach
- Use `map[string]customSpec` where the value type implements `UnmarshalFlag`
- Parse compact specs: `--customs id:url=...,model=...,api-key=...`
- Support environment variables: `CUSTOM_<ID>_<FIELD>`
- Maintain backward compatibility with existing `--custom.*` flags

### CLI Syntax Examples
```bash
# Single custom provider
mpt --customs openrouter:url=https://openrouter.ai/api/v1,model=claude-3.5-sonnet,api-key=$KEY \
    --prompt "Hello"

# Multiple providers
mpt --customs openrouter:url=https://openrouter.ai/api/v1,model=claude-3.5-sonnet \
    --customs local:url=http://localhost:11434,model=mixtral \
    --customs perplexity:url=https://api.perplexity.ai,model=llama-3.1-sonar \
    --prompt "Compare approaches"

# With mix mode
mpt --customs llm1:url=http://llm1.local,model=model1 \
    --customs llm2:url=http://llm2.local,model=model2 \
    --mix --mix.provider=llm1
```

### Environment Variable Examples
```bash
# OpenRouter configuration
export CUSTOM_OPENROUTER_URL="https://openrouter.ai/api/v1"
export CUSTOM_OPENROUTER_API_KEY="sk-or-..."
export CUSTOM_OPENROUTER_MODEL="anthropic/claude-3.5-sonnet"
export CUSTOM_OPENROUTER_TEMPERATURE="0.3"

# Local Ollama
export CUSTOM_LOCAL_URL="http://localhost:11434"
export CUSTOM_LOCAL_MODEL="mixtral:8x7b"

# Run with environment-configured providers
mpt --prompt "Hello"
```

## Implementation Details

### Phase 1: Core Data Structures

#### 1.1 Add customSpec Type
```go
// cmd/mpt/main.go

// customSpec represents a parsed custom provider specification
type customSpec struct {
    Name        string
    URL         string  
    APIKey      string
    Model       string
    MaxTokens   SizeValue
    Temperature float32
    Enabled     bool
}

// UnmarshalFlag parses "url=https://...,model=xxx,api-key=xxx" format
func (c *customSpec) UnmarshalFlag(value string) error {
    // Set defaults
    c.Temperature = 0.7
    c.MaxTokens = 16384
    c.Enabled = true  // enabled by default; override with enabled=false
    
    // Parse comma-separated key=value pairs
    pairs := strings.Split(value, ",")
    for _, pair := range pairs {
        kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
        if len(kv) != 2 {
            return fmt.Errorf("invalid format in '%s' (expected key=value)", pair)
        }
        
        key := strings.ToLower(strings.TrimSpace(kv[0]))
        val := strings.TrimSpace(kv[1])
        
        switch key {
        // URL aliases
        case "url", "base-url", "base_url", "baseurl":
            c.URL = val
            
        // API key aliases
        case "api-key", "api_key", "apikey":
            c.APIKey = val
            
        case "model":
            c.Model = val
            
        case "name":
            c.Name = val
            
        // Max tokens aliases
        case "max-tokens", "max_tokens", "maxtokens":
            var sv SizeValue
            if err := sv.UnmarshalFlag(val); err != nil {
                return fmt.Errorf("invalid max-tokens '%s': %w", val, err)
            }
            c.MaxTokens = sv
            
        // Temperature aliases
        case "temperature", "temp":
            temp, err := strconv.ParseFloat(val, 32)
            if err != nil {
                return fmt.Errorf("invalid temperature '%s': %w", val, err)
            }
            if temp < 0 || temp > 2 {
                return fmt.Errorf("temperature must be between 0 and 2, got %f", temp)
            }
            c.Temperature = float32(temp)
            
        case "enabled":
            enabled, err := strconv.ParseBool(val)
            if err != nil {
                return fmt.Errorf("invalid enabled value '%s': %w", val, err)
            }
            c.Enabled = enabled
            
        default:
            // Warning instead of error for forward compatibility
            lgr.Printf("[WARN] unknown key '%s' in custom provider spec (ignoring)", key)
        }
    }
    
    return nil
}
```

#### 1.2 Add ID Validation Helpers
```go
// validateProviderID ensures ID contains only [a-z0-9-_]
func validateProviderID(id string) error {
    if id == "" {
        return fmt.Errorf("provider ID cannot be empty")
    }
    
    // Check for valid characters
    for _, r := range id {
        if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
            return fmt.Errorf("provider ID '%s' contains invalid character '%c' (use only a-z, 0-9, -, _)", id, r)
        }
    }
    
    return nil
}

// normalizeProviderID converts ID to lowercase for consistency
func normalizeProviderID(id string) string {
    return strings.ToLower(strings.TrimSpace(id))
}
```

#### 1.3 Update options Struct
```go
type options struct {
    OpenAI    openAIOpts    `group:"openai" namespace:"openai" env-namespace:"OPENAI"`
    Anthropic anthropicOpts `group:"anthropic" namespace:"anthropic" env-namespace:"ANTHROPIC"`
    Google    googleOpts    `group:"google" namespace:"google" env-namespace:"GOOGLE"`
    
    // Keep legacy single custom for backward compatibility
    Custom customOpenAIProvider `group:"custom" namespace:"custom" env-namespace:"CUSTOM"`
    
    // New map for multiple custom providers
    Customs map[string]customSpec `long:"customs" description:"Add custom OpenAI-compatible provider as 'id:key=value[,key=value,...]' (e.g., openrouter:url=https://openrouter.ai/api/v1,model=claude-3.5)" key-value-delimiter:":" value-name:"ID:SPEC"`
    
    // ... rest of existing fields ...
}
```

### Phase 2: Environment Variable Support

#### 2.1 Create Environment Parser
```go
// parseCustomProvidersFromEnv scans environment for CUSTOM_<ID>_<FIELD> patterns
func parseCustomProvidersFromEnv() (map[string]customSpec, []string) {
    providers := make(map[string]customSpec)
    envMap := make(map[string]map[string]string) // id -> field -> value
    var warnings []string
    
    // Legacy single custom env vars to skip
    legacyVars := map[string]bool{
        "CUSTOM_URL": true,
        "CUSTOM_API_KEY": true,
        "CUSTOM_MODEL": true,
        "CUSTOM_MAX_TOKENS": true,
        "CUSTOM_TEMPERATURE": true,
        "CUSTOM_ENABLED": true,
        "CUSTOM_NAME": true,
    }
    
    // Collect all CUSTOM_* environment variables
    for _, env := range os.Environ() {
        parts := strings.SplitN(env, "=", 2)
        if len(parts) != 2 {
            continue
        }
        
        key := parts[0]
        value := parts[1]
        
        // Skip if not CUSTOM_ prefix or is legacy var
        if !strings.HasPrefix(key, "CUSTOM_") || legacyVars[key] {
            continue
        }
        
        // Parse ID and field from CUSTOM_<ID>_<FIELD>
        remaining := strings.TrimPrefix(key, "CUSTOM_")
        parts = strings.SplitN(remaining, "_", 2)
        if len(parts) != 2 {
            continue
        }
        
        id := normalizeProviderID(parts[0])
        field := strings.ToLower(parts[1])
        
        // Validate ID
        if err := validateProviderID(id); err != nil {
            warnings = append(warnings, fmt.Sprintf("skipping env var %s: %v", key, err))
            continue
        }
        
        if envMap[id] == nil {
            envMap[id] = make(map[string]string)
        }
        envMap[id][field] = value
    }
    
    // Convert to customSpec
    for id, fields := range envMap {
        spec := customSpec{
            Name:        id, // default name to ID
            Temperature: 0.7,
            MaxTokens:   16384,
            Enabled:     true,
        }
        
        for field, value := range fields {
            switch field {
            case "url", "base_url":
                spec.URL = value
                
            case "api_key", "apikey":
                spec.APIKey = value
                
            case "model":
                spec.Model = value
                
            case "name":
                spec.Name = value
                
            case "max_tokens", "maxtokens":
                if err := spec.MaxTokens.UnmarshalFlag(value); err != nil {
                    warnings = append(warnings, 
                        fmt.Sprintf("custom[%s]: invalid max_tokens '%s': %v", id, value, err))
                }
                
            case "temperature", "temp":
                if temp, err := strconv.ParseFloat(value, 32); err == nil {
                    if temp >= 0 && temp <= 2 {
                        spec.Temperature = float32(temp)
                    } else {
                        warnings = append(warnings,
                            fmt.Sprintf("custom[%s]: temperature %f out of range [0,2]", id, temp))
                    }
                } else {
                    warnings = append(warnings,
                        fmt.Sprintf("custom[%s]: invalid temperature '%s': %v", id, value, err))
                }
                
            case "enabled":
                if enabled, err := strconv.ParseBool(value); err == nil {
                    spec.Enabled = enabled
                } else {
                    warnings = append(warnings,
                        fmt.Sprintf("custom[%s]: invalid enabled value '%s': %v", id, value, err))
                }
            }
        }
        
        providers[id] = spec
    }
    
    return providers, warnings
}
```

### Phase 3: Provider Initialization

#### 3.1 Update initializeProviders
```go
func initializeProviders(opts *options) ([]provider.Provider, error) {
    var providers []provider.Provider
    var errors []string
    
    // ... existing standard providers code (OpenAI, Anthropic, Google) ...
    
    // Initialize custom providers
    customProviders, customErrors := initializeCustomProviders(opts)
    providers = append(providers, customProviders...)
    errors = append(errors, customErrors...)
    
    if len(providers) == 0 && len(errors) > 0 {
        return nil, fmt.Errorf("all enabled providers failed to initialize:\n%s", strings.Join(errors, "\n"))
    }
    
    return providers, nil
}
```

#### 3.2 Create initializeCustomProviders
```go
func initializeCustomProviders(opts *options) ([]provider.Provider, []string) {
    var providers []provider.Provider
    var errors []string
    
    // Build merged customs map with proper precedence
    customs := make(map[string]customSpec)
    
    // 1. Start with environment providers (lowest precedence)
    envProviders, envWarnings := parseCustomProvidersFromEnv()
    for _, warning := range envWarnings {
        lgr.Printf("[WARN] %s", warning)
    }
    for id, spec := range envProviders {
        customs[id] = spec
        lgr.Printf("[DEBUG] added custom provider from env: %s", id)
    }
    
    // 2. Add legacy custom if configured (middle precedence)
    if opts.Custom.Enabled || opts.Custom.URL != "" {
        id := "custom"
        if opts.Custom.Name != "" {
            id = normalizeProviderID(opts.Custom.Name)
        }
        
        customs[id] = customSpec{
            Name:        opts.Custom.Name,
            URL:         opts.Custom.URL,
            APIKey:      opts.Custom.APIKey,
            Model:       opts.Custom.Model,
            MaxTokens:   opts.Custom.MaxTokens,
            Temperature: opts.Custom.Temperature,
            Enabled:     true,
        }
        
        lgr.Printf("[DEBUG] converted legacy --custom.* to customs[%s]", id)
    }
    
    // 3. Apply CLI customs map (highest precedence - overwrites)
    for id, spec := range opts.Customs {
        normalizedID := normalizeProviderID(id)
        if err := validateProviderID(normalizedID); err != nil {
            errors = append(errors, err.Error())
            continue
        }
        customs[normalizedID] = spec
        if id != normalizedID {
            lgr.Printf("[DEBUG] normalized custom provider ID: %s -> %s", id, normalizedID)
        }
    }
    
    // Sort IDs for deterministic processing
    var ids []string
    for id := range customs {
        ids = append(ids, id)
    }
    sort.Strings(ids)
    
    // Create providers in sorted order
    for _, id := range ids {
        spec := customs[id]
        
        if !spec.Enabled {
            lgr.Printf("[DEBUG] skipping disabled custom provider: %s", id)
            continue
        }
        
        // Validate required fields
        if spec.URL == "" {
            errors = append(errors, fmt.Sprintf("custom[%s]: missing URL", id))
            continue
        }
        if spec.Model == "" {
            errors = append(errors, fmt.Sprintf("custom[%s]: missing model", id))
            continue
        }
        
        // Set name if not specified
        if spec.Name == "" {
            spec.Name = id
        }
        
        // Create provider
        p := provider.NewCustomOpenAI(provider.CustomOptions{
            Name:        spec.Name,
            BaseURL:     spec.URL,
            APIKey:      spec.APIKey,
            Model:       spec.Model,
            Enabled:     true,
            MaxTokens:   int(spec.MaxTokens),
            Temperature: spec.Temperature,
        })
        
        providers = append(providers, p)
        lgr.Printf("[DEBUG] initialized custom provider: %s (id: %s), URL: %s, model: %s, temp: %.2f", 
            spec.Name, id, spec.URL, spec.Model, spec.Temperature)
    }
    
    return providers, errors
}
```

### Phase 4: Helper Functions Updates

#### 4.1 Update collectSecrets
```go
func collectSecrets(opts *options) []string {
    secretsMap := make(map[string]bool) // Use map to avoid duplicates
    
    // ... existing code for standard providers ...
    
    // Build effective customs map (same precedence as init)
    customs := make(map[string]customSpec)
    
    // Environment (lowest precedence)
    envProviders, _ := parseCustomProvidersFromEnv()
    for id, spec := range envProviders {
        customs[id] = spec
    }
    
    // Legacy (middle precedence)
    if opts.Custom.APIKey != "" {
        id := "custom"
        if opts.Custom.Name != "" {
            id = normalizeProviderID(opts.Custom.Name)
        }
        customs[id] = customSpec{APIKey: opts.Custom.APIKey}
    }
    
    // CLI map (highest precedence)
    for id, spec := range opts.Customs {
        normalizedID := normalizeProviderID(id)
        customs[normalizedID] = spec
    }
    
    // Collect unique secrets
    for _, spec := range customs {
        if spec.APIKey != "" {
            secretsMap[spec.APIKey] = true
        }
    }
    
    // Convert to slice
    var secrets []string
    for secret := range secretsMap {
        secrets = append(secrets, secret)
    }
    
    return secrets
}
```

#### 4.2 Update anyProvidersEnabled
```go
func anyProvidersEnabled(opts *options) bool {
    // Check standard providers
    if opts.OpenAI.Enabled || opts.Anthropic.Enabled || opts.Google.Enabled {
        return true
    }
    
    // Check legacy custom
    if opts.Custom.Enabled {
        return true
    }
    
    // Check new customs map
    if len(opts.Customs) > 0 {
        return true
    }
    
    // Check environment custom providers
    envProviders, _ := parseCustomProvidersFromEnv()
    if len(envProviders) > 0 {
        return true
    }
    
    return false
}
```

### Phase 5: Testing

#### 5.1 Unit Tests for customSpec
```go
func TestCustomSpecUnmarshalFlag(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected customSpec
        wantErr  bool
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
            name:  "minimal spec",
            input: "url=http://localhost:8080,model=local-llm",
            expected: customSpec{
                URL:         "http://localhost:8080",
                Model:       "local-llm",
                Temperature: 0.7,  // default
                MaxTokens:   16384, // default
                Enabled:     true,  // default
            },
        },
        {
            name:  "with aliases",
            input: "base-url=http://test.com,api_key=key,max_tokens=4k,temp=0.3",
            expected: customSpec{
                URL:         "http://test.com",
                APIKey:      "key",
                MaxTokens:   4096,
                Temperature: 0.3,
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
            name:    "invalid format - missing equals",
            input:   "url=test,invalid",
            wantErr: true,
        },
        {
            name:    "invalid temperature",
            input:   "url=test,model=test,temperature=abc",
            wantErr: true,
        },
        {
            name:    "temperature out of range",
            input:   "url=test,model=test,temperature=3.0",
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var spec customSpec
            err := spec.UnmarshalFlag(tt.input)
            
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.expected, spec)
        })
    }
}
```

#### 5.2 Environment Parsing Tests
```go
func TestParseCustomProvidersFromEnv(t *testing.T) {
    // Set test environment
    os.Setenv("CUSTOM_OPENROUTER_URL", "https://openrouter.ai/api/v1")
    os.Setenv("CUSTOM_OPENROUTER_MODEL", "claude-3.5-sonnet")
    os.Setenv("CUSTOM_OPENROUTER_API_KEY", "secret-key")
    os.Setenv("CUSTOM_LOCAL_URL", "http://localhost:11434")
    os.Setenv("CUSTOM_LOCAL_MODEL", "mixtral")
    os.Setenv("CUSTOM_LOCAL_TEMPERATURE", "0.5")
    
    // Should be ignored (legacy)
    os.Setenv("CUSTOM_URL", "http://legacy.com")
    os.Setenv("CUSTOM_MODEL", "legacy-model")
    
    defer func() {
        os.Unsetenv("CUSTOM_OPENROUTER_URL")
        os.Unsetenv("CUSTOM_OPENROUTER_MODEL")
        os.Unsetenv("CUSTOM_OPENROUTER_API_KEY")
        os.Unsetenv("CUSTOM_LOCAL_URL")
        os.Unsetenv("CUSTOM_LOCAL_MODEL")
        os.Unsetenv("CUSTOM_LOCAL_TEMPERATURE")
        os.Unsetenv("CUSTOM_URL")
        os.Unsetenv("CUSTOM_MODEL")
    }()
    
    providers, warnings := parseCustomProvidersFromEnv()
    
    require.Len(t, providers, 2)
    assert.Len(t, warnings, 0)
    
    // Check OpenRouter
    assert.Equal(t, "https://openrouter.ai/api/v1", providers["openrouter"].URL)
    assert.Equal(t, "claude-3.5-sonnet", providers["openrouter"].Model)
    assert.Equal(t, "secret-key", providers["openrouter"].APIKey)
    
    // Check Local
    assert.Equal(t, "http://localhost:11434", providers["local"].URL)
    assert.Equal(t, "mixtral", providers["local"].Model)
    assert.Equal(t, float32(0.5), providers["local"].Temperature)
}
```

#### 5.3 Additional Test Cases
```go
func TestCustomProvidersOverride(t *testing.T) {
    // Test that last --customs wins for same ID
}

func TestPrecedenceOrder(t *testing.T) {
    // Test ENV < Legacy < CLI precedence
}

func TestIDNormalization(t *testing.T) {
    // Test ID validation and normalization
}

func TestDisabledProviderNotInitialized(t *testing.T) {
    // Test that disabled providers are skipped
}

func TestAnyProvidersEnabledWithEnvOnly(t *testing.T) {
    // Test that env-only customs return true
}
```

### Phase 6: Documentation Updates

#### README.md Additions
- New section: "Multiple Custom Providers"
- Examples for CLI usage with multiple providers
- Environment variable configuration examples
- Precedence rules explanation
- Migration guide from single to multiple

#### Help Text Updates
- Update `--customs` description with inline example
- Note about quoting specs with spaces/commas
- Reference to environment variable support

## Implementation Checklist

- [ ] Phase 1: Core Data Structures
  - [ ] Implement `customSpec` type with `UnmarshalFlag`
  - [ ] Add ID validation and normalization helpers
  - [ ] Update `options` struct with `Customs` map field

- [ ] Phase 2: Environment Variable Support
  - [ ] Implement `parseCustomProvidersFromEnv`
  - [ ] Handle legacy variable skipping
  - [ ] Add proper error handling and warnings

- [ ] Phase 3: Provider Initialization
  - [ ] Update `initializeProviders` function
  - [ ] Create `initializeCustomProviders` with precedence handling
  - [ ] Add deterministic processing with sorted IDs

- [ ] Phase 4: Helper Functions
  - [ ] Update `collectSecrets` for deduplication
  - [ ] Update `anyProvidersEnabled` to check all sources

- [ ] Phase 5: Testing
  - [ ] Unit tests for `customSpec.UnmarshalFlag`
  - [ ] Environment parsing tests
  - [ ] Precedence order tests
  - [ ] ID normalization tests
  - [ ] Integration tests

- [ ] Phase 6: Documentation
  - [ ] Update README with examples
  - [ ] Add migration guide
  - [ ] Update help text
  - [ ] Document precedence rules

## Key Design Decisions

1. **Flag name**: `--customs` (plural) to avoid collision with legacy `--custom.*`
2. **Precedence**: CLI > Legacy > Environment (clear and predictable)
3. **ID format**: Lowercase letters, numbers, hyphens, underscores `[a-z0-9-_]`
4. **Unknown keys**: Warn but don't error (forward compatibility)
5. **Defaults**: Temperature=0.7, MaxTokens=16384, Enabled=true
6. **Error handling**: Aggregate errors, continue with valid providers
7. **Determinism**: Sort IDs for consistent processing order

## Backward Compatibility

- Legacy `--custom.*` flags continue to work unchanged
- Legacy environment variables (`CUSTOM_URL`, etc.) continue to work
- Internally converted to map entry with ID from name or "custom"
- No breaking changes for existing users

## Testing Strategy

1. **Unit tests**: Parse logic, validation, normalization
2. **Integration tests**: Full provider initialization flow
3. **Edge cases**: Override behavior, disabled providers, invalid IDs
4. **Backward compatibility**: Legacy flags still work

## Migration Path for Users

1. Current single custom provider continues to work
2. Can gradually adopt new `--customs` syntax
3. Can mix legacy and new approaches during transition
4. Clear documentation with examples

## Success Criteria

- [x] Multiple custom providers can be configured via CLI
- [x] Environment variable configuration works
- [x] Backward compatibility maintained
- [x] Clean, intuitive syntax
- [x] Comprehensive test coverage
- [x] Clear documentation and examples