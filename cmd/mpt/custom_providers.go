package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/provider"
)

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
	// set defaults
	c.Temperature = 0.7
	c.MaxTokens = 16384
	c.Enabled = true // enabled by default; override with enabled=false

	// parse comma-separated key=value pairs
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid format in '%s' (expected key=value)", pair)
		}

		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])

		switch key {
		// url aliases
		case "url", "base-url", "base_url", "baseurl":
			c.URL = val

		// api key aliases
		case "api-key", "api_key", "apikey":
			c.APIKey = val

		case "model":
			c.Model = val

		case "name":
			c.Name = val

		// max tokens aliases
		case "max-tokens", "max_tokens", "maxtokens":
			var sv SizeValue
			if err := sv.UnmarshalFlag(val); err != nil {
				return fmt.Errorf("invalid max-tokens '%s': %w", val, err)
			}
			c.MaxTokens = sv

		// temperature aliases
		case "temperature", "temp":
			temp, err := strconv.ParseFloat(val, 32)
			if err != nil {
				return fmt.Errorf("invalid temperature '%s': %w", val, err)
			}
			if temp < 0 || temp > 2 {
				return fmt.Errorf("temperature must be between 0 and 2, got %g", temp)
			}
			c.Temperature = float32(temp)

		case "enabled":
			enabled, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("invalid enabled value '%s': %w", val, err)
			}
			c.Enabled = enabled

		default:
			// warning instead of error for forward compatibility
			lgr.Printf("[WARN] unknown key '%s' in custom provider spec (ignoring)", key)
		}
	}

	return nil
}

// validateProviderID ensures ID contains only [a-z0-9-_]
func validateProviderID(id string) error {
	if id == "" {
		return fmt.Errorf("provider ID cannot be empty")
	}

	// check for valid characters
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return fmt.Errorf("provider ID '%s' contains invalid character '%c' (use only a-z, 0-9, -, _)", id, r)
		}
	}

	return nil
}

// normalizeProviderID converts ID to lowercase for consistency
func normalizeProviderID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// parseCustomProvidersFromEnv scans environment for CUSTOM_<ID>_<FIELD> patterns
func parseCustomProvidersFromEnv() (providers map[string]customSpec, warnings []string) {
	providers = make(map[string]customSpec)
	envMap := make(map[string]map[string]string) // id -> field -> value

	// legacy single custom env vars to skip
	legacyVars := map[string]bool{
		"CUSTOM_URL":         true,
		"CUSTOM_API_KEY":     true,
		"CUSTOM_MODEL":       true,
		"CUSTOM_MAX_TOKENS":  true,
		"CUSTOM_TEMPERATURE": true,
		"CUSTOM_ENABLED":     true,
		"CUSTOM_NAME":        true,
	}

	// collect all CUSTOM_* environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// skip if not CUSTOM_ prefix or is legacy var
		if !strings.HasPrefix(key, "CUSTOM_") || legacyVars[key] {
			continue
		}

		// parse ID and field from CUSTOM_<ID>_<FIELD>
		remaining := strings.TrimPrefix(key, "CUSTOM_")
		parts = strings.SplitN(remaining, "_", 2)
		if len(parts) != 2 {
			continue
		}

		id := normalizeProviderID(parts[0])
		field := strings.ToLower(parts[1])

		// validate ID
		if err := validateProviderID(id); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping env var %s: %v", key, err))
			continue
		}

		if envMap[id] == nil {
			envMap[id] = make(map[string]string)
		}
		envMap[id][field] = value
	}

	// convert to customSpec
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
							fmt.Sprintf("custom[%s]: temperature %g out of range [0,2]", id, temp))
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

// initializeCustomProviders initializes custom providers with proper precedence
func initializeCustomProviders(opts *options) (providers []provider.Provider, errors []string) {
	providers = make([]provider.Provider, 0, 10)
	errors = make([]string, 0, 5)

	// build merged customs map with proper precedence
	customs := make(map[string]customSpec)

	// 1. start with environment providers (lowest precedence)
	envProviders, envWarnings := parseCustomProvidersFromEnv()
	for _, warning := range envWarnings {
		lgr.Printf("[WARN] %s", warning)
	}
	for id, spec := range envProviders {
		customs[id] = spec
		lgr.Printf("[DEBUG] added custom provider from env: %s", id)
	}

	// 2. add legacy custom if configured (middle precedence)
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

	// 3. apply CLI customs map (highest precedence - overwrites)
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

	// sort IDs for deterministic processing
	ids := make([]string, 0, len(customs))
	for id := range customs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// create providers in sorted order
	for _, id := range ids {
		spec := customs[id]

		if !spec.Enabled {
			lgr.Printf("[DEBUG] skipping disabled custom provider: %s", id)
			continue
		}

		// validate required fields
		if spec.URL == "" {
			errors = append(errors, fmt.Sprintf("custom[%s]: missing URL", id))
			continue
		}
		if spec.Model == "" {
			errors = append(errors, fmt.Sprintf("custom[%s]: missing model", id))
			continue
		}

		// set name if not specified
		if spec.Name == "" {
			spec.Name = id
		}

		// create provider
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

// collectCustomSecrets collects API keys from all custom provider sources
func collectCustomSecrets(opts *options) []string {
	secretsMap := make(map[string]bool) // use map to avoid duplicates

	// build effective customs map (same precedence as init)
	customs := make(map[string]customSpec)

	// environment (lowest precedence)
	envProviders, _ := parseCustomProvidersFromEnv()
	for id, spec := range envProviders {
		customs[id] = spec
	}

	// legacy (middle precedence)
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

	// collect unique secrets
	for _, spec := range customs {
		if spec.APIKey != "" {
			secretsMap[spec.APIKey] = true
		}
	}

	// convert to slice
	secrets := make([]string, 0, len(secretsMap))
	for secret := range secretsMap {
		secrets = append(secrets, secret)
	}

	return secrets
}

// anyCustomProvidersEnabled checks if any custom providers are enabled
func anyCustomProvidersEnabled(opts *options) bool {
	// check legacy custom
	if opts.Custom.Enabled {
		return true
	}

	// check new customs map
	if len(opts.Customs) > 0 {
		return true
	}

	// check environment custom providers
	envProviders, _ := parseCustomProvidersFromEnv()
	return len(envProviders) > 0
}
