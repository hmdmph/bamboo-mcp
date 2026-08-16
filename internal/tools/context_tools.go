package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	pctx "github.com/hmdmph/bamboo-mcp/internal/context"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/mark3labs/mcp-go/mcp"
)

// ContextTools provides MCP tools that leverage plan context knowledge
// to intelligently resolve plans and deployment statuses.
type ContextTools struct {
	client  *bamboo.Client
	context *pctx.PlanContext
}

// NewContextTools creates a new ContextTools instance.
func NewContextTools(client *bamboo.Client, planContext *pctx.PlanContext) *ContextTools {
	return &ContextTools{
		client:  client,
		context: planContext,
	}
}

// GetPlanContext returns the current plan context knowledge/configuration.
func (t *ContextTools) GetPlanContext(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_plan_context")

	summary := t.context.GetContextSummary()

	content, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// ReloadContext reloads the plan context configuration from disk.
func (t *ContextTools) ReloadContext(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_reload_context")

	if err := t.context.Reload(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to reload context: %v", err)), nil
	}

	summary := t.context.GetContextSummary()
	content, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("✅ Context reloaded successfully.\n\n%s", string(content))), nil
}

// ResolvePlan resolves a human-friendly reference to a full Bamboo plan key,
// then searches for it and returns the plan details.
func (t *ContextTools) ResolvePlan(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_resolve_plan args=%v", arguments)

	ref, _ := arguments["reference"].(string)
	if ref == "" {
		return mcp.NewToolResultError("reference is required (e.g., 'CHECKOUT', 'PAYMENTS')"), nil
	}

	planType, _ := arguments["planType"].(string)

	planKey, resolvedType, err := t.context.ResolvePlanKey(ref, planType)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve plan key: %v", err)), nil
	}

	logger.Info("Resolved plan key: %s (type: %s)", planKey, resolvedType)

	// Try to get the plan directly
	plan, err := t.client.GetPlan(planKey)
	if err != nil {
		// Fall back to search
		logger.Info("Direct plan lookup failed, trying search for: %s", ref)
		searchResult, searchErr := t.client.SearchPlans(ref)
		if searchErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan key '%s' not found and search failed: %v", planKey, searchErr)), nil
		}

		result := map[string]interface{}{
			"resolved_plan_key": planKey,
			"resolved_type":     resolvedType,
			"direct_lookup":     "failed",
			"search_results":    searchResult,
			"hint":              "The resolved plan key was not found. Check the search results for matching plans.",
		}

		content, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(content)), nil
	}

	result := map[string]interface{}{
		"resolved_plan_key": planKey,
		"resolved_type":     resolvedType,
		"plan":              plan,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// GetDeployStatus is a smart deployment status tool. Given a reference name, plan type,
// and optional environment/module filters, it resolves the plan, finds the deployment project,
// retrieves latest build status, and filters relevant environments.
func (t *ContextTools) GetDeployStatus(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_deploy_status args=%v", arguments)

	ref, _ := arguments["reference"].(string)
	if ref == "" {
		return mcp.NewToolResultError("reference is required (e.g., 'CHECKOUT', 'PAYMENTS')"), nil
	}

	planType, _ := arguments["planType"].(string)
	envFilter, _ := arguments["environment"].(string)
	moduleFilter, _ := arguments["module"].(string)

	// Step 1: Resolve the plan key
	planKey, resolvedType, err := t.context.ResolvePlanKey(ref, planType)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve plan key: %v", err)), nil
	}
	logger.Info("Resolved plan key: %s (type: %s)", planKey, resolvedType)

	// Step 2: Get latest build result
	latestResult, err := t.client.GetLatestResult(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get latest build result for '%s': %v", planKey, err)), nil
	}

	// Step 3: Get deployment projects for this plan
	deployProjects, err := t.client.ListDeploymentProjectsForPlan(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list deployment projects for '%s': %v", planKey, err)), nil
	}

	// Build Bamboo UI links
	bambooBaseURL := t.client.GetBaseURL()

	// Step 4: Get deployment project details and filter environments
	var deploymentDetails []map[string]interface{}

	for _, dp := range deployProjects {
		dpMap, ok := dp.(map[string]interface{})
		if !ok {
			continue
		}

		dpID := ""
		if id, ok := dpMap["id"].(float64); ok {
			dpID = fmt.Sprintf("%.0f", id)
		}

		if dpID == "" {
			continue
		}

		dpDetail, err := t.client.GetDeploymentProject(dpID)
		if err != nil {
			logger.Error("Failed to get deployment project %s: %v", dpID, err)
			continue
		}

		// Extract and filter environments
		envs, _ := dpDetail["environments"].([]interface{})
		var filteredEnvs []map[string]interface{}
		var allEnvNames []string

		for _, e := range envs {
			envMap, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			envName, _ := envMap["name"].(string)
			allEnvNames = append(allEnvNames, envName)

			// Apply filters
			if envFilter != "" && !strings.Contains(strings.ToLower(envName), strings.ToLower(envFilter)) {
				continue
			}
			if moduleFilter != "" && !strings.Contains(strings.ToLower(envName), strings.ToLower(moduleFilter)) {
				continue
			}

			envInfo := map[string]interface{}{
				"name":     envName,
				"id":       envMap["id"],
				"position": envMap["position"],
			}

			// Fetch latest deployment result for this environment
			envID := ""
			if eid, ok := envMap["id"].(float64); ok {
				envID = fmt.Sprintf("%.0f", eid)
			}
			if envID != "" {
				envResults, err := t.client.GetDeploymentEnvironmentResults(envID, 1)
				if err == nil {
					latestDeploy := extractLatestDeployResult(envResults, bambooBaseURL)
					if latestDeploy != nil {
						envInfo["latest_deployment"] = latestDeploy
					}
				} else {
					logger.Error("Failed to get env results for %s: %v", envID, err)
				}
			}

			filteredEnvs = append(filteredEnvs, envInfo)
		}

		dpName, _ := dpDetail["name"].(string)
		detail := map[string]interface{}{
			"deployment_project_id":   dpID,
			"deployment_project_name": dpName,
			"total_environments":      len(allEnvNames),
			"matched_environments":    filteredEnvs,
		}

		deploymentDetails = append(deploymentDetails, detail)
	}

	buildNumber := ""
	if bn, ok := latestResult["buildNumber"].(float64); ok {
		buildNumber = fmt.Sprintf("%.0f", bn)
	}

	result := map[string]interface{}{
		"plan_key":     planKey,
		"plan_type":    resolvedType,
		"latest_build": extractBuildSummary(latestResult),
		"deployments":  deploymentDetails,
		"links": map[string]string{
			"plan":         fmt.Sprintf("%s/browse/%s", bambooBaseURL, planKey),
			"latest_build": fmt.Sprintf("%s/browse/%s-%s", bambooBaseURL, planKey, buildNumber),
		},
	}

	if envFilter != "" || moduleFilter != "" {
		result["filters_applied"] = map[string]string{
			"environment": envFilter,
			"module":      moduleFilter,
		}
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// ExplainEnvironment parses a deployment environment name into its components
// using plan context knowledge.
func (t *ContextTools) ExplainEnvironment(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_explain_environment args=%v", arguments)

	envName, _ := arguments["environmentName"].(string)
	if envName == "" {
		return mcp.NewToolResultError("environmentName is required (e.g., 'staging_network_checkout_deploy')"), nil
	}

	planType, _ := arguments["planType"].(string)

	parsed := t.context.ParseEnvironmentName(envName, planType)

	// Enrich with plan type info if available
	if planType != "" {
		pt := t.context.GetPlanTypeByName(planType)
		if pt != nil {
			parsed["plan_type"] = pt.Type
			parsed["plan_type_description"] = pt.Description
			parsed["environment_format"] = pt.EnvFormat
		}
	}

	result := map[string]interface{}{
		"environment_name": envName,
		"parsed":           parsed,
	}

	if planType == "" {
		result["hint"] = "Provide planType (app, infra) for better parsing"
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// GetPlanTypeInfo returns detailed information about a specific plan type.
func (t *ContextTools) GetPlanTypeInfo(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_plan_type_info args=%v", arguments)

	planType, _ := arguments["planType"].(string)
	if planType == "" {
		return mcp.NewToolResultError("planType is required (app, infra)"), nil
	}

	pt := t.context.GetPlanTypeByName(planType)
	if pt == nil {
		// List available types
		available := []string{}
		for _, p := range t.context.GetPlanTypes() {
			available = append(available, p.Type)
		}
		return mcp.NewToolResultError(fmt.Sprintf("Unknown plan type '%s'. Available: %s", planType, strings.Join(available, ", "))), nil
	}

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

	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// extractBuildSummary extracts key fields from a build result.
func extractBuildSummary(build map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}

	fields := []string{
		"buildNumber", "buildState", "buildResultKey", "buildRelativeTime",
		"buildCompletedTime", "buildDurationDescription", "buildReason",
		"successful", "lifeCycleState",
	}
	for _, f := range fields {
		if v, ok := build[f]; ok {
			summary[f] = v
		}
	}

	return summary
}

// extractLatestDeployResult extracts the latest deployment result from environment results.
// Returns a summary with status, who deployed, when, version, and link to logs.
func extractLatestDeployResult(envResults map[string]interface{}, bambooBaseURL string) map[string]interface{} {
	results, ok := envResults["results"].([]interface{})
	if !ok || len(results) == 0 {
		return nil
	}

	latest, ok := results[0].(map[string]interface{})
	if !ok {
		return nil
	}

	summary := map[string]interface{}{}

	// Deployment state/status
	if v, ok := latest["deploymentState"].(string); ok {
		summary["status"] = v
	}
	if v, ok := latest["lifeCycleState"].(string); ok {
		summary["lifecycle_state"] = v
	}

	// Deployment result ID
	resultID := ""
	if v, ok := latest["id"].(float64); ok {
		resultID = fmt.Sprintf("%.0f", v)
		summary["deployment_result_id"] = resultID
	}

	// Who triggered the deployment
	if v, ok := latest["reasonSummary"].(string); ok {
		summary["triggered_by"] = v
	}

	// Start and finish times
	if v, ok := latest["startedDate"].(float64); ok {
		summary["started_date"] = v
	}
	if v, ok := latest["finishedDate"].(float64); ok {
		summary["finished_date"] = v
	}
	if v, ok := latest["queuedDate"].(float64); ok {
		summary["queued_date"] = v
	}
	if v, ok := latest["executedDate"].(float64); ok {
		summary["executed_date"] = v
	}

	// Deployment version info
	if dv, ok := latest["deploymentVersion"].(map[string]interface{}); ok {
		versionInfo := map[string]interface{}{}
		if name, ok := dv["name"].(string); ok {
			versionInfo["name"] = name
		}
		if id, ok := dv["id"].(float64); ok {
			versionInfo["id"] = fmt.Sprintf("%.0f", id)
		}
		if creatorName, ok := dv["creatorDisplayName"].(string); ok {
			versionInfo["creator"] = creatorName
		}
		summary["version"] = versionInfo
	}

	// Build link to deployment result logs
	if resultID != "" && bambooBaseURL != "" {
		summary["log_url"] = fmt.Sprintf("%s/deploy/viewDeploymentResult.action?deploymentResultId=%s", bambooBaseURL, resultID)
	}

	return summary
}
