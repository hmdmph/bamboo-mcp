package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"gopkg.in/yaml.v3"
)

// ActionDetail describes a deployment/build action with its purpose.
// Supports unmarshalling from both a plain string ("synth") and a struct ({name: synth, description: "..."}).
type ActionDetail struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

func (a *ActionDetail) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try plain string first
	var s string
	if err := unmarshal(&s); err == nil {
		a.Name = s
		return nil
	}
	// Fall back to struct
	type raw ActionDetail
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*a = ActionDetail(r)
	return nil
}

// LogPattern describes a pattern found in build/deployment logs that indicates a specific outcome.
type LogPattern struct {
	Name      string `yaml:"name"`
	Pattern   string `yaml:"pattern"`
	Indicates string `yaml:"indicates"`
}

// CustomInput describes a custom variable that can be passed when running a plan.
type CustomInput struct {
	Variable    string   `yaml:"variable"`
	Description string   `yaml:"description"`
	Values      []string `yaml:"values,omitempty"`
	Default     string   `yaml:"default,omitempty"`
}

// PlanType defines a type of plan based on naming conventions with rich domain knowledge.
type PlanType struct {
	Suffix               string            `yaml:"suffix"`
	Type                 string            `yaml:"type"`
	Description          string            `yaml:"description"`
	HasBranch            bool              `yaml:"has_branch"`
	PlanKeyMatch         string            `yaml:"plan_key_match,omitempty"`
	PlanNameFormat       string            `yaml:"plan_name_format"`
	EnvFormat            string            `yaml:"environment_format,omitempty"`
	Environments         []string          `yaml:"environments,omitempty"`
	CustomEnvironments   bool              `yaml:"custom_environments,omitempty"`
	Modules              []string          `yaml:"modules,omitempty"`
	ModuleDescriptions   map[string]string `yaml:"module_descriptions,omitempty"`
	Actions              []ActionDetail    `yaml:"actions,omitempty"`
	LogPatterns          []LogPattern      `yaml:"log_patterns,omitempty"`
	CustomInputs         []CustomInput     `yaml:"custom_inputs,omitempty"`
	FixedDeployments     []string          `yaml:"fixed_deployments,omitempty"`
	DeploymentProjectFmt string            `yaml:"deployment_project_format,omitempty"`
	Hints                []string          `yaml:"hints,omitempty"`
}

// ContextConfig holds the full context knowledge configuration.
type ContextConfig struct {
	DefaultProject string     `yaml:"default_project"`
	GenericHints   []string   `yaml:"generic_hints,omitempty"`
	PlanTypes      []PlanType `yaml:"plan_types"`
}

// PlanContext manages the plan naming context knowledge.
type PlanContext struct {
	mu       sync.RWMutex
	config   *ContextConfig
	filePath string
}

// defaultConfig returns the built-in starter context configuration.
//
// These values are deliberately generic placeholders. The plan context feature
// only becomes useful once you describe *your own* Bamboo conventions: edit
// ~/.bamboo-mcp/context.yaml (written on first run) and call
// bamboo_reload_context. See examples/context.yaml for a fully worked example.
func defaultConfig() *ContextConfig {
	return &ContextConfig{
		DefaultProject: "EXAMPLE",
		GenericHints: []string{
			"This is the default starter context — edit ~/.bamboo-mcp/context.yaml to describe your own Bamboo conventions",
			"default_project is the Bamboo project key that plan keys are resolved against",
			"Plan names typically follow: 'Project - <REF> - <Type>' or 'Project - <REF> (<branch>) - <Type>'",
			"Deployment environment names usually encode the target environment and an action",
		},
		PlanTypes: []PlanType{
			appPlanType(),
			infraPlanType(),
		},
	}
}

func appPlanType() PlanType {
	return PlanType{
		Suffix:             "App",
		Type:               "app",
		Description:        "Application deployments built from a branch and released to one or more environments.",
		HasBranch:          true,
		PlanNameFormat:     "Project - APP<REF> (<branch>) - App",
		EnvFormat:          "<env>_<action>",
		Environments:       []string{"dev", "staging", "prod"},
		CustomEnvironments: true,
		Actions: []ActionDetail{
			{Name: "deploy", Description: "Deploy the application to the target environment"},
			{Name: "rollback", Description: "Roll the application back to a previous release"},
			{Name: "inspect", Description: "Inspect the currently deployed state"},
		},
		Hints: []string{
			"(<branch>) in the plan name indicates the branch being deployed (e.g. main, develop)",
			"Set custom_environments: true if your teams use environment names beyond the standard list",
		},
	}
}

func infraPlanType() PlanType {
	return PlanType{
		Suffix:         "Infra",
		Type:           "infra",
		Description:    "Infrastructure deployments, where each environment targets a specific module.",
		HasBranch:      false,
		PlanNameFormat: "Project - <REF> - Infra",
		EnvFormat:      "<env>_<module>_<ref>_<action>",
		Environments:   []string{"dev", "staging", "prod"},
		Modules:        []string{"network", "database", "storage"},
		ModuleDescriptions: map[string]string{
			"network":  "Networking resources (VPCs, subnets, load balancers)",
			"database": "Managed database instances and clusters",
			"storage":  "Object storage and file systems",
		},
		Actions: []ActionDetail{
			{Name: "plan", Description: "Preview the changes that would be applied"},
			{Name: "deploy", Description: "Apply the infrastructure changes"},
			{Name: "teardown", Description: "Destroy the provisioned infrastructure"},
		},
		Hints: []string{
			"Environment names embed the module, so bamboo_explain_environment can decompose them",
			"List your own modules under 'modules' so they can be matched and described",
		},
	}
}

// NewPlanContext creates a new PlanContext. It loads from the YAML file if it exists,
// otherwise uses the embedded defaults and writes the default config to disk.
func NewPlanContext(configPath string) (*PlanContext, error) {
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".bamboo-mcp", "context.yaml")
	}

	pc := &PlanContext{
		filePath: configPath,
	}

	if err := pc.load(); err != nil {
		if os.IsNotExist(err) {
			logger.Info("Context config not found at %s, using defaults and creating file", configPath)
			pc.config = defaultConfig()
			if saveErr := pc.save(); saveErr != nil {
				logger.Error("Failed to save default context config: %v", saveErr)
			}
		} else {
			return nil, fmt.Errorf("failed to load context config: %w", err)
		}
	}

	logger.Info("Loaded plan context: default_project=%s, plan_types=%d", pc.config.DefaultProject, len(pc.config.PlanTypes))
	return pc, nil
}

func (pc *PlanContext) load() error {
	data, err := os.ReadFile(pc.filePath)
	if err != nil {
		return err
	}

	var cfg ContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse context config: %w", err)
	}

	pc.config = &cfg
	return nil
}

func (pc *PlanContext) save() error {
	dir := filepath.Dir(pc.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(pc.config)
	if err != nil {
		return fmt.Errorf("failed to marshal context config: %w", err)
	}

	return os.WriteFile(pc.filePath, data, 0600)
}

// GetConfig returns the current context configuration.
func (pc *PlanContext) GetConfig() *ContextConfig {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.config
}

// GetDefaultProject returns the default Bamboo project key.
func (pc *PlanContext) GetDefaultProject() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.config.DefaultProject
}

// GetPlanTypes returns all configured plan types.
func (pc *PlanContext) GetPlanTypes() []PlanType {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.config.PlanTypes
}

// ResolvePlanKey attempts to resolve a plan key from a reference name and optional type.
// For example: ref="CHECKOUT", planType="app" -> "EXAMPLE-CHECKOUTAPP"
func (pc *PlanContext) ResolvePlanKey(ref string, planType string) (string, string, error) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	ref = strings.TrimSpace(strings.ToUpper(ref))
	planType = strings.TrimSpace(strings.ToLower(planType))

	for _, pt := range pc.config.PlanTypes {
		if pt.PlanKeyMatch != "" && planType == pt.Type {
			return pt.PlanKeyMatch, pt.Type, nil
		}
	}

	if planType != "" {
		suffix := pc.suffixForType(planType)
		if suffix != "" {
			planKey := fmt.Sprintf("%s-%s%s", pc.config.DefaultProject, ref, strings.ToUpper(strings.ReplaceAll(suffix, " ", "")))
			return planKey, planType, nil
		}
		return "", "", fmt.Errorf("unknown plan type: %s", planType)
	}

	// If no type specified, try exact match first
	planKey := fmt.Sprintf("%s-%s", pc.config.DefaultProject, ref)
	return planKey, "", nil
}

// ResolveEnvironmentName constructs a deployment environment name from its components.
// env="staging", module="network", ref="checkout", action="deploy"
// -> "staging_network_checkout_deploy"
func (pc *PlanContext) ResolveEnvironmentName(env, module, ref, action string) string {
	parts := []string{
		strings.TrimSpace(strings.ToLower(env)),
		strings.TrimSpace(strings.ToLower(module)),
		strings.TrimSpace(strings.ToLower(ref)),
		strings.TrimSpace(strings.ToLower(action)),
	}
	return strings.Join(parts, "_")
}

// MatchEnvironments filters deployment environments by partial matching.
// Returns environment names that contain the filter string.
func (pc *PlanContext) MatchEnvironments(environments []string, filter string) []string {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return environments
	}

	var matched []string
	for _, env := range environments {
		if strings.Contains(strings.ToLower(env), filter) {
			matched = append(matched, env)
		}
	}
	return matched
}

// GetPlanTypeByName returns the PlanType matching the given type name, or nil.
func (pc *PlanContext) GetPlanTypeByName(typeName string) *PlanType {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	typeName = strings.ToLower(strings.TrimSpace(typeName))
	for i, pt := range pc.config.PlanTypes {
		if strings.EqualFold(pt.Type, typeName) {
			return &pc.config.PlanTypes[i]
		}
	}
	return nil
}

// formatFields extracts the ordered placeholder names from an environment_format
// string such as "<env>_<module>_<ref>_<action>" -> ["env","module","ref","action"].
// Literal (non-placeholder) segments are returned as-is so they can be matched.
func formatFields(format string) []string {
	if format == "" {
		return nil
	}
	return strings.Split(strings.ToLower(strings.TrimSpace(format)), "_")
}

// parseByFormat maps the underscore-separated parts of an environment name onto
// the placeholders declared in the plan type's environment_format.
//
// The <module> placeholder is greedy because module names may themselves contain
// underscores (e.g. "load_balancer"); it is matched against the plan type's known
// modules, longest candidate first. All other placeholders consume one segment,
// except the final one which absorbs whatever remains.
func parseByFormat(pt *PlanType, parts []string, result map[string]string) {
	fields := formatFields(pt.EnvFormat)
	if len(fields) == 0 {
		result["environment"] = parts[0]
		if len(parts) > 1 {
			result["remaining"] = strings.Join(parts[1:], "_")
		}
		return
	}

	pos := 0
	for i, f := range fields {
		if pos >= len(parts) {
			return
		}
		name := strings.Trim(f, "<>")
		last := i == len(fields)-1

		// Literal segment in the format (e.g. the "nodegroup" in
		// "<env>_nodegroup_<action>") — record it and move on.
		if !strings.HasPrefix(f, "<") {
			if parts[pos] == name {
				result["marker"] = name
				pos++
			}
			continue
		}

		if name == "module" && len(pt.Modules) > 0 {
			matched := false
			for tryLen := len(parts) - pos; tryLen >= 1; tryLen-- {
				candidate := strings.Join(parts[pos:pos+tryLen], "_")
				for _, m := range pt.Modules {
					if candidate == m {
						result["module"] = candidate
						if desc, ok := pt.ModuleDescriptions[candidate]; ok {
							result["module_description"] = desc
						}
						pos += tryLen
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				result["remaining"] = strings.Join(parts[pos:], "_")
				return
			}
			continue
		}

		key := name
		if name == "env" {
			key = "environment"
		}
		if last {
			result[key] = strings.Join(parts[pos:], "_")
			pos = len(parts)
		} else {
			result[key] = parts[pos]
			pos++
		}
	}

	if pos < len(parts) {
		result["remaining"] = strings.Join(parts[pos:], "_")
	}
}

// ParseEnvironmentName parses a deployment environment name into its components
// using the known format for a given plan type. Returns a map of component names to values.
func (pc *PlanContext) ParseEnvironmentName(envName string, planTypeName string) map[string]string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	result := map[string]string{"raw": envName}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(envName)), "_")
	if len(parts) == 0 {
		return result
	}

	var pt *PlanType
	for i, p := range pc.config.PlanTypes {
		if strings.EqualFold(p.Type, planTypeName) {
			pt = &pc.config.PlanTypes[i]
			break
		}
	}
	if pt == nil {
		result["environment"] = parts[0]
		if len(parts) > 1 {
			result["remaining"] = strings.Join(parts[1:], "_")
		}
		return result
	}

	// A plan type with fixed deployment names has no positional format to parse.
	if len(pt.FixedDeployments) > 0 {
		result["deployment_name"] = envName
		for _, fd := range pt.FixedDeployments {
			if strings.EqualFold(envName, fd) {
				result["matched_fixed_deployment"] = fd
				break
			}
		}
	} else {
		// Otherwise parse positionally against the declared environment_format,
		// e.g. "<env>_<module>_<ref>_<action>". Unknown formats fall back to
		// splitting off the leading environment segment.
		parseByFormat(pt, parts, result)
	}

	// Look up action description if we have an action
	if actionName, ok := result["action"]; ok && pt != nil {
		for _, a := range pt.Actions {
			if strings.EqualFold(a.Name, actionName) {
				result["action_description"] = a.Description
				break
			}
		}
	}

	return result
}

// GetContextSummary returns a comprehensive summary of the plan context knowledge.
func (pc *PlanContext) GetContextSummary() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	planTypes := make([]map[string]interface{}, 0, len(pc.config.PlanTypes))
	for _, pt := range pc.config.PlanTypes {
		info := map[string]interface{}{
			"type":             pt.Type,
			"suffix":           pt.Suffix,
			"description":      pt.Description,
			"has_branch":       pt.HasBranch,
			"plan_name_format": pt.PlanNameFormat,
		}
		if pt.PlanKeyMatch != "" {
			info["plan_key_match"] = pt.PlanKeyMatch
		}
		if pt.EnvFormat != "" {
			info["environment_format"] = pt.EnvFormat
		}
		if len(pt.Environments) > 0 {
			info["environments"] = pt.Environments
		}
		if pt.CustomEnvironments {
			info["custom_environments"] = true
		}
		if len(pt.Modules) > 0 {
			info["modules"] = pt.Modules
		}
		if len(pt.ModuleDescriptions) > 0 {
			info["module_descriptions"] = pt.ModuleDescriptions
		}
		if len(pt.Actions) > 0 {
			actions := make([]map[string]string, 0, len(pt.Actions))
			for _, a := range pt.Actions {
				actions = append(actions, map[string]string{"name": a.Name, "description": a.Description})
			}
			info["actions"] = actions
		}
		if len(pt.LogPatterns) > 0 {
			patterns := make([]map[string]string, 0, len(pt.LogPatterns))
			for _, lp := range pt.LogPatterns {
				patterns = append(patterns, map[string]string{
					"name": lp.Name, "pattern": lp.Pattern, "indicates": lp.Indicates,
				})
			}
			info["log_patterns"] = patterns
		}
		if len(pt.CustomInputs) > 0 {
			inputs := make([]map[string]interface{}, 0, len(pt.CustomInputs))
			for _, ci := range pt.CustomInputs {
				inp := map[string]interface{}{
					"variable":    ci.Variable,
					"description": ci.Description,
				}
				if len(ci.Values) > 0 {
					inp["values"] = ci.Values
				}
				if ci.Default != "" {
					inp["default"] = ci.Default
				}
				inputs = append(inputs, inp)
			}
			info["custom_inputs"] = inputs
		}
		if len(pt.FixedDeployments) > 0 {
			info["fixed_deployments"] = pt.FixedDeployments
		}
		if pt.DeploymentProjectFmt != "" {
			info["deployment_project_format"] = pt.DeploymentProjectFmt
		}
		if len(pt.Hints) > 0 {
			info["hints"] = pt.Hints
		}
		planTypes = append(planTypes, info)
	}

	summary := map[string]interface{}{
		"default_project": pc.config.DefaultProject,
		"plan_types":      planTypes,
		"config_file":     pc.filePath,
	}
	if len(pc.config.GenericHints) > 0 {
		summary["generic_hints"] = pc.config.GenericHints
	}
	summary["usage_hints"] = []string{
		fmt.Sprintf("Most deployment plans are under the '%s' project", pc.config.DefaultProject),
		"Use bamboo_resolve_plan to convert a reference name to a full plan key",
		"Use bamboo_get_deploy_status to get deployment status with smart environment filtering",
		"Use bamboo_explain_environment to parse an environment name into its components",
		"Edit the context.yaml file to customize plan types and naming conventions",
	}
	return summary
}

func (pc *PlanContext) suffixForType(planType string) string {
	for _, pt := range pc.config.PlanTypes {
		if strings.EqualFold(pt.Type, planType) {
			return pt.Suffix
		}
	}
	return ""
}

// Reload reloads the context configuration from disk.
func (pc *PlanContext) Reload() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	data, err := os.ReadFile(pc.filePath)
	if err != nil {
		return err
	}

	var cfg ContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse context config: %w", err)
	}

	pc.config = &cfg
	logger.Info("Reloaded plan context from %s", pc.filePath)
	return nil
}
