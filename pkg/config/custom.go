package config

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/provider"
)

const defaultCustomMaxTokens = 16384

// CustomSpec represents a parsed custom provider specification
type CustomSpec struct {
	Name         string
	URL          string
	APIKey       string
	Model        string
	MaxTokens    int
	Temperature  float32
	EndpointType string
	Enabled      bool
}

// CustomProviderManager manages custom provider configuration and initialization
type CustomProviderManager struct {
	cliCustoms   map[string]CustomSpec
	legacyCustom *CustomSpec
}

// NewCustomProviderManager creates a new custom provider manager
func NewCustomProviderManager(cliCustoms map[string]CustomSpec, legacyCustom *CustomSpec) *CustomProviderManager {
	return &CustomProviderManager{
		cliCustoms:   cliCustoms,
		legacyCustom: legacyCustom,
	}
}

// InitializeProviders initializes all custom providers with proper precedence.
// It merges provider configurations from three sources (in order of precedence):
//  1. Environment variables (CUSTOM_<ID>_<FIELD>) - lowest precedence
//  2. Legacy --custom.* flags - middle precedence
//  3. CLI --customs map - highest precedence (overwrites previous)
func (m *CustomProviderManager) InitializeProviders() (providers []provider.Provider, errors []string) {
	providers = make([]provider.Provider, 0, 10)
	errors = make([]string, 0, 5)

	// build merged customs map with proper precedence
	customs, warnings := m.buildEffectiveCustomsMap()
	for _, warning := range warnings {
		lgr.Printf("[WARN] %s", warning)
		if !strings.Contains(warning, "skipping env var") {
			// non-env warnings are also errors to be returned
			errors = append(errors, warning)
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

		// validate required fields and always log errors
		if spec.URL == "" {
			err := fmt.Sprintf("custom[%s]: missing URL", id)
			errors = append(errors, err)
			lgr.Printf("[WARN] %s", err)
			continue
		}
		if spec.Model == "" {
			err := fmt.Sprintf("custom[%s]: missing model", id)
			errors = append(errors, err)
			lgr.Printf("[WARN] %s", err)
			continue
		}

		// set name if not specified
		if spec.Name == "" {
			spec.Name = id
		}

		// create provider
		p := provider.NewCustomOpenAI(provider.CustomOptions{
			Name:         spec.Name,
			BaseURL:      spec.URL,
			APIKey:       spec.APIKey,
			Model:        spec.Model,
			Enabled:      true,
			MaxTokens:    spec.MaxTokens,
			Temperature:  spec.Temperature,
			EndpointType: provider.EndpointType(spec.EndpointType),
		})

		providers = append(providers, p)

		// log with proper temperature display
		tempDisplay := fmt.Sprintf("%.2f", spec.Temperature)
		if spec.Temperature < 0 {
			tempDisplay = "(default)"
		}
		lgr.Printf("[DEBUG] initialized custom provider: %s (id: %s), URL: %s, model: %s, temp: %s",
			spec.Name, id, spec.URL, spec.Model, tempDisplay)
	}

	return providers, errors
}

// CollectSecrets collects all unique API keys from custom provider sources
func (m *CustomProviderManager) CollectSecrets() []string {
	secretsMap := make(map[string]bool) // use map to avoid duplicates

	// build effective customs map using shared function
	customs, _ := m.buildEffectiveCustomsMap()

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

// AnyEnabled checks if any custom providers are enabled
func (m *CustomProviderManager) AnyEnabled() bool {
	// build the effective customs map with all precedence rules applied
	customs, _ := m.buildEffectiveCustomsMap()

	// check if any provider is actually enabled
	for _, spec := range customs {
		if spec.Enabled {
			return true
		}
	}

	return false
}

// buildEffectiveCustomsMap builds the merged customs map with proper precedence.
// Precedence order (lowest to highest): environment variables, legacy custom flags, CLI customs map
func (m *CustomProviderManager) buildEffectiveCustomsMap() (customs map[string]CustomSpec, warnings []string) {
	customs = make(map[string]CustomSpec)

	// 1. start with environment providers (lowest precedence)
	envProviders, envWarnings := m.parseCustomProvidersFromEnv()
	warnings = append(warnings, envWarnings...)
	for id, spec := range envProviders {
		customs[id] = spec
		lgr.Printf("[DEBUG] added custom provider from env: %s", id)
	}

	// 2. add legacy custom if configured (middle precedence)
	if m.legacyCustom != nil && m.legacyCustom.Enabled {
		id := "custom"
		if m.legacyCustom.Name != "" {
			id = normalizeProviderID(m.legacyCustom.Name)
		}

		spec := *m.legacyCustom
		customs[id] = spec
		lgr.Printf("[DEBUG] converted legacy --custom.* to customs[%s]", id)
	}

	// 3. apply CLI customs map (highest precedence - overwrites)
	for id, spec := range m.cliCustoms {
		normalizedID := normalizeProviderID(id)
		if err := validateProviderID(normalizedID); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		customs[normalizedID] = spec
		if id != normalizedID {
			lgr.Printf("[DEBUG] normalized custom provider ID: %s -> %s", id, normalizedID)
		}
	}

	return customs, warnings
}

// parseCustomProvidersFromEnv scans environment for CUSTOM_<ID>_<FIELD> patterns
func (m *CustomProviderManager) parseCustomProvidersFromEnv() (providers map[string]CustomSpec, warnings []string) {
	providers = make(map[string]CustomSpec)
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

		// parse ID and field from CUSTOM_<ID>_<FIELD> using suffix matching
		remaining := strings.TrimPrefix(key, "CUSTOM_")

		// known field suffixes (all use underscores to match env var convention)
		knownFields := []string{
			"_endpoint_type",
			"_max_tokens",
			"_api_key",
			"_temperature",
			"_enabled",
			"_model",
			"_name",
			"_url",
		}

		var id, field string
		found := false
		lowerRemaining := strings.ToLower(remaining)

		// try to match known field suffixes
		for _, suffix := range knownFields {
			if !strings.HasSuffix(lowerRemaining, suffix) {
				continue
			}
			// extract ID (everything before the suffix)
			idEnd := len(remaining) - len(suffix)
			if idEnd <= 0 {
				warnings = append(warnings, fmt.Sprintf("skipping env var %s: empty provider ID", key))
				found = true
				break
			}
			id = normalizeProviderID(remaining[:idEnd])
			field = strings.TrimPrefix(suffix, "_")
			found = true
			break
		}

		if !found {
			warnings = append(warnings, fmt.Sprintf("skipping env var %s: unrecognized field name (valid fields: url, api_key, model, name, max_tokens, temperature, endpoint_type, enabled)", key))
			continue
		}

		if id == "" {
			continue // already warned above
		}

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

	// convert to CustomSpec
	for id, fields := range envMap {
		spec := CustomSpec{
			Name:         id, // default name to ID
			Temperature:  -1, // -1 means unset, will use provider default
			MaxTokens:    defaultCustomMaxTokens,
			EndpointType: "chat_completions", // default to chat_completions for custom providers
			Enabled:      false,              // disabled by default, matches standard providers
		}

		for field, value := range fields {
			fieldWarnings := applyEnvField(&spec, id, field, value)
			warnings = append(warnings, fieldWarnings...)
		}

		providers[id] = spec
	}

	return providers, warnings
}

// applyEnvField applies a single environment variable field to a CustomSpec and returns any warnings
func applyEnvField(spec *CustomSpec, id, field, value string) []string {
	var warnings []string

	switch field {
	case "url":
		spec.URL = value

	case "api_key":
		spec.APIKey = value

	case "model":
		spec.Model = value

	case "name":
		spec.Name = value

	case "max_tokens":
		if tokens, err := ParseSize(value); err == nil {
			// safe downcast with overflow check
			if tokens > math.MaxInt32 {
				warnings = append(warnings,
					fmt.Sprintf("custom[%s]: max_tokens value too large", id))
			} else {
				spec.MaxTokens = int(tokens)
			}
		} else {
			warnings = append(warnings,
				fmt.Sprintf("custom[%s]: invalid max_tokens '%s': %v", id, value, err))
		}

	case "temperature":
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

	case "endpoint_type":
		// validate endpoint type
		valueLower := strings.ToLower(value)
		if valueLower == "auto" || valueLower == "responses" || valueLower == "chat_completions" {
			spec.EndpointType = valueLower
		} else {
			warnings = append(warnings,
				fmt.Sprintf("custom[%s]: invalid endpoint_type '%s' (valid: auto, responses, chat_completions)", id, value))
		}

	case "enabled":
		if enabled, err := strconv.ParseBool(value); err == nil {
			spec.Enabled = enabled
		} else {
			warnings = append(warnings,
				fmt.Sprintf("custom[%s]: invalid enabled value '%s': %v", id, value, err))
		}
	}

	return warnings
}

// ParseCustomSpec parses "url=https://...,model=xxx,api-key=xxx" format string into CustomSpec.
// This is used for parsing CLI flag values.
func ParseCustomSpec(value string) (CustomSpec, error) {
	spec := CustomSpec{
		// set defaults (-1 means unset for temperature to allow explicit 0)
		Temperature:  -1, // will be set to default by provider if not specified
		MaxTokens:    defaultCustomMaxTokens,
		EndpointType: "chat_completions", // default to chat_completions for custom providers
		Enabled:      false,              // disabled by default, matches standard providers
	}

	// parse comma-separated key=value pairs
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			return spec, fmt.Errorf("invalid format in '%s' (expected key=value)", pair)
		}

		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])

		switch key {
		case "url":
			spec.URL = val

		case "api-key":
			spec.APIKey = val

		case "model":
			spec.Model = val

		case "name":
			spec.Name = val

		case "max-tokens":
			tokens, err := ParseSize(val)
			if err != nil {
				return spec, fmt.Errorf("invalid max-tokens '%s': %w", val, err)
			}
			// safe downcast with overflow check
			if tokens > math.MaxInt32 {
				return spec, fmt.Errorf("max-tokens value too large")
			}
			spec.MaxTokens = int(tokens)

		case "temperature":
			temp, err := strconv.ParseFloat(val, 32)
			if err != nil {
				return spec, fmt.Errorf("invalid temperature '%s': %w", val, err)
			}
			if temp < 0 || temp > 2 {
				return spec, fmt.Errorf("temperature must be between 0 and 2, got %g", temp)
			}
			spec.Temperature = float32(temp)

		case "endpoint-type":
			// validate endpoint type
			valLower := strings.ToLower(val)
			if valLower != "auto" && valLower != "responses" && valLower != "chat_completions" {
				return spec, fmt.Errorf("invalid endpoint-type '%s' (valid: auto, responses, chat_completions)", val)
			}
			spec.EndpointType = valLower

		case "enabled":
			enabled, err := strconv.ParseBool(val)
			if err != nil {
				return spec, fmt.Errorf("invalid enabled value '%s': %w", val, err)
			}
			spec.Enabled = enabled

		default:
			// warning instead of error for forward compatibility
			lgr.Printf("[WARN] unknown key '%s' in custom provider spec (ignoring)", key)
		}
	}

	return spec, nil
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
