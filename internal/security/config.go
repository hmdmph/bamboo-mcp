package security

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// ToolPolicy defines per-tool security constraints.
type ToolPolicy struct {
	// AllowedMethods restricts which HTTP methods the tool may use downstream.
	// Default: ["GET"]. Non-GET calls are always rejected unless explicitly listed.
	AllowedMethods []string `yaml:"allowed_methods"`

	// RequireEnvironment forces an explicit "environment" argument for this tool.
	RequireEnvironment bool `yaml:"require_environment"`

	// AllowedEnvironments restricts which environment values are accepted (empty = all).
	AllowedEnvironments []string `yaml:"allowed_environments"`

	// DeniedEnvironments blocks specific environments (e.g. prod for broad-scan tools).
	DeniedEnvironments []string `yaml:"denied_environments"`

	// MaxResults caps numeric result/pagination arguments (0 = no cap).
	MaxResults int `yaml:"max_results"`

	// RateLimitPerMinute caps calls per minute for this tool (0 = use default).
	RateLimitPerMinute int `yaml:"rate_limit_per_minute"`

	// Disabled prevents this tool from running at all.
	Disabled bool `yaml:"disabled"`
}

// SecurityConfig is the top-level configuration for the security layer.
type SecurityConfig struct {
	// RequireExplicitEnvironment forces every env-sensitive tool to receive an
	// explicit "environment" argument. Environment is NEVER inferred from context.
	RequireExplicitEnvironment bool `yaml:"require_explicit_environment"`

	// EnvSensitiveTools lists tools that must have an explicit environment argument.
	EnvSensitiveTools []string `yaml:"env_sensitive_tools"`

	// DefaultAllowedMethods is the fallback allowed HTTP method list for all tools.
	// Default: ["GET"]. POST/PUT/PATCH/DELETE are blocked unless listed here.
	DefaultAllowedMethods []string `yaml:"default_allowed_methods"`

	// DefaultRateLimitPerMinute is the default per-tool call cap (0 = unlimited).
	DefaultRateLimitPerMinute int `yaml:"default_rate_limit_per_minute"`

	// SanitizeResponses enables prompt-injection scanning on every API response.
	// Detected patterns are stripped/redacted and the content is labeled UNTRUSTED.
	SanitizeResponses bool `yaml:"sanitize_responses"`

	// MaxResponseSizeBytes truncates any single API response larger than this
	// to prevent large-scale data exfiltration through the model context (0 = no limit).
	MaxResponseSizeBytes int `yaml:"max_response_size_bytes"`

	// ToolPolicies maps tool names to their individual security policies.
	ToolPolicies map[string]ToolPolicy `yaml:"tool_policies"`
}

var defaultConfig = &SecurityConfig{
	RequireExplicitEnvironment: false,
	SanitizeResponses:          true,
	DefaultAllowedMethods:      []string{"GET"},
	DefaultRateLimitPerMinute:  120,
	MaxResponseSizeBytes:       512 * 1024, // 512 KB
	EnvSensitiveTools: []string{
		"bamboo_get_deploy_status",
		"bamboo_explain_environment",
	},
	ToolPolicies: map[string]ToolPolicy{
		// Broad-export tools get tighter rate limits
		"bamboo_list_all_projects": {
			AllowedMethods:     []string{"GET"},
			MaxResults:         200,
			RateLimitPerMinute: 10,
		},
		"bamboo_list_build_results": {
			AllowedMethods:     []string{"GET"},
			MaxResults:         100,
			RateLimitPerMinute: 30,
		},
		"bamboo_get_environment_results": {
			AllowedMethods:     []string{"GET"},
			MaxResults:         50,
			RateLimitPerMinute: 30,
		},
		"bamboo_list_deploy_versions": {
			AllowedMethods:     []string{"GET"},
			MaxResults:         50,
			RateLimitPerMinute: 30,
		},
		// Smart deployment status: environment is required
		"bamboo_get_deploy_status": {
			AllowedMethods: []string{"GET"},
		},
	},
}

// securityConfigHolder is the loaded config protected by a mutex for hot-reload.
type securityConfigHolder struct {
	mu       sync.RWMutex
	cfg      *SecurityConfig
	filePath string
}

var globalHolder = &securityConfigHolder{}

// LoadSecurityConfig loads the security config from filePath, falling back to defaults.
// If filePath is empty, uses ~/.bamboo-mcp/security.yaml; if the file does not exist,
// defaults are used and the file is created.
func LoadSecurityConfig(filePath string) (*SecurityConfig, error) {
	if filePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return defaultConfig, nil
		}
		filePath = filepath.Join(home, ".bamboo-mcp", "security.yaml")
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err2 := saveDefault(filePath); err2 != nil {
			// Non-fatal: just use in-memory defaults
			return copyDefault(), nil
		}
		return copyDefault(), nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	cfg := copyDefault()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Always enforce GET-only as the floor
	if len(cfg.DefaultAllowedMethods) == 0 {
		cfg.DefaultAllowedMethods = []string{"GET"}
	}

	globalHolder.mu.Lock()
	globalHolder.cfg = cfg
	globalHolder.filePath = filePath
	globalHolder.mu.Unlock()

	return cfg, nil
}

// Reload re-reads the config file.
func Reload() (*SecurityConfig, error) {
	globalHolder.mu.RLock()
	path := globalHolder.filePath
	globalHolder.mu.RUnlock()
	return LoadSecurityConfig(path)
}

func copyDefault() *SecurityConfig {
	policies := make(map[string]ToolPolicy, len(defaultConfig.ToolPolicies))
	for k, v := range defaultConfig.ToolPolicies {
		policies[k] = v
	}
	envTools := make([]string, len(defaultConfig.EnvSensitiveTools))
	copy(envTools, defaultConfig.EnvSensitiveTools)
	methods := make([]string, len(defaultConfig.DefaultAllowedMethods))
	copy(methods, defaultConfig.DefaultAllowedMethods)

	return &SecurityConfig{
		RequireExplicitEnvironment: defaultConfig.RequireExplicitEnvironment,
		SanitizeResponses:          defaultConfig.SanitizeResponses,
		DefaultAllowedMethods:      methods,
		DefaultRateLimitPerMinute:  defaultConfig.DefaultRateLimitPerMinute,
		MaxResponseSizeBytes:       defaultConfig.MaxResponseSizeBytes,
		EnvSensitiveTools:          envTools,
		ToolPolicies:               policies,
	}
}

func saveDefault(filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0600)
}
