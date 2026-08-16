package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/mark3labs/mcp-go/mcp"
)

type BambooTools struct {
	client       *bamboo.Client
	bitbucketURL string
}

func NewBambooTools(client *bamboo.Client, bitbucketURL string) *BambooTools {
	return &BambooTools{client: client, bitbucketURL: strings.TrimRight(bitbucketURL, "/")}
}

// repoInfo holds parsed Bitbucket repository details extracted from a Bamboo VCS revision name.
// Bamboo repo name format: PLATFORM.{project}.{repo-name}.{branch}.app
type repoInfo struct {
	Name    string `json:"name"`
	Project string `json:"project"`
	Branch  string `json:"branch"`
	Commit  string `json:"commit"`
	RepoURL string `json:"repo_url"`
	DiffURL string `json:"commit_url"`
}

// parseHelmRepo parses a Bamboo VCS repository name in the format
// PLATFORM.{project}.{repo-name}.{branch}.app and returns a repoInfo.
// Returns nil if the name does not match the helm chart pattern.
func parseHelmRepo(repoName, vcsRevision, bitbucketURL string) *repoInfo {
	parts := strings.Split(repoName, ".")
	if len(parts) < 5 {
		return nil
	}
	if parts[len(parts)-1] != "app" {
		return nil
	}
	project := strings.ToUpper(parts[1])
	repo := parts[2]
	branch := strings.Join(parts[3:len(parts)-1], ".")

	repoURL := ""
	commitURL := ""
	if bitbucketURL != "" {
		repoURL = fmt.Sprintf("%s/projects/%s/repos/%s/browse", bitbucketURL, project, repo)
		commitURL = fmt.Sprintf("%s/projects/%s/repos/%s/commits/%s", bitbucketURL, project, repo, vcsRevision)
	}

	return &repoInfo{
		Name:    repo,
		Project: project,
		Branch:  branch,
		Commit:  vcsRevision,
		RepoURL: repoURL,
		DiffURL: commitURL,
	}
}

func (t *BambooTools) GetServerInfo(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_server_info")
	info, err := t.client.GetServerInfo()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get server info: %v", err)), nil
	}

	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) HealthCheck(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_health_check")
	healthy, err := t.client.HealthCheck()

	var status string
	if healthy {
		status = "✅ Bamboo server is healthy and accessible"
	} else {
		status = fmt.Sprintf("❌ Bamboo server health check failed: %v", err)
	}

	return mcp.NewToolResultText(status), nil
}

func (t *BambooTools) ListProjects(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_projects args=%v", arguments)

	maxResult := 25
	if mr, ok := arguments["maxResult"].(float64); ok {
		maxResult = int(mr)
	}
	startIndex := 0
	if si, ok := arguments["startIndex"].(float64); ok {
		startIndex = int(si)
	}

	projects, err := t.client.ListProjects(maxResult, startIndex)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list projects: %v", err)), nil
	}

	content, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListAllProjects(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_all_projects")

	allProjects, err := t.client.ListAllProjects()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list all projects: %v", err)), nil
	}

	result := map[string]interface{}{
		"totalCount": len(allProjects),
		"projects":   allProjects,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetProject(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_project args=%v", arguments)
	projectKey, ok := arguments["projectKey"].(string)
	if !ok || projectKey == "" {
		return mcp.NewToolResultError("projectKey is required"), nil
	}

	project, err := t.client.GetProject(projectKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get project: %v", err)), nil
	}

	content, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListPlans(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_plans")
	plans, err := t.client.ListPlans()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list plans: %v", err)), nil
	}

	content, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetPlan(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_plan args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	plan, err := t.client.GetPlan(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get plan: %v", err)), nil
	}

	content, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) SearchPlans(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_search_plans args=%v", arguments)
	searchTerm, ok := arguments["searchTerm"].(string)
	if !ok || searchTerm == "" {
		return mcp.NewToolResultError("searchTerm is required"), nil
	}

	plans, err := t.client.SearchPlans(searchTerm)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search plans: %v", err)), nil
	}

	content, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListPlanBranches(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_plan_branches args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	branches, err := t.client.ListPlanBranches(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list plan branches: %v", err)), nil
	}

	content, err := json.MarshalIndent(branches, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetPlanBranch(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_plan_branch args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	branchName, ok := arguments["branchName"].(string)
	if !ok || branchName == "" {
		return mcp.NewToolResultError("branchName is required"), nil
	}

	branch, err := t.client.GetPlanBranch(planKey, branchName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get plan branch: %v", err)), nil
	}

	content, err := json.MarshalIndent(branch, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetLatestResult(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_latest_result args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	result, err := t.client.GetLatestResult(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get latest result: %v", err)), nil
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListBuildResults(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_build_results args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	maxResults := 25
	if mr, ok := arguments["maxResults"].(float64); ok {
		maxResults = int(mr)
	}

	results, err := t.client.ListBuildResults(planKey, maxResults)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list build results: %v", err)), nil
	}

	content, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetBuildResult(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_build_result args=%v", arguments)
	buildKey, ok := arguments["buildKey"].(string)
	if !ok || buildKey == "" {
		return mcp.NewToolResultError("buildKey is required"), nil
	}

	result, err := t.client.GetBuildResult(buildKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get build result: %v", err)), nil
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetBuildResultExpanded(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_build_result_expanded args=%v", arguments)
	buildKey, ok := arguments["buildKey"].(string)
	if !ok || buildKey == "" {
		return mcp.NewToolResultError("buildKey is required"), nil
	}

	result, err := t.client.GetBuildResultExpanded(buildKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get expanded build result: %v", err)), nil
	}

	// Add convenience log URL
	baseURL := t.client.GetBaseURL()
	if baseURL != "" {
		result["_links"] = map[string]string{
			"log_view": fmt.Sprintf("%s/browse/%s/log", baseURL, buildKey),
			"build":    fmt.Sprintf("%s/browse/%s", baseURL, buildKey),
		}
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetBuildRepositories(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_build_repositories args=%v", arguments)
	buildKey, ok := arguments["buildKey"].(string)
	if !ok || buildKey == "" {
		return mcp.NewToolResultError("buildKey is required"), nil
	}

	raw, err := t.client.GetBuildRepositories(buildKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get build repositories: %v", err)), nil
	}

	// Parse and enrich VCS revisions into helm chart repositories.
	var repos []repoInfo
	if vcsRevisions, ok := raw["vcsRevisions"].(map[string]interface{}); ok {
		if revList, ok := vcsRevisions["vcsRevision"].([]interface{}); ok {
			for _, item := range revList {
				rev, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := rev["repositoryName"].(string)
				commit, _ := rev["vcsRevisionKey"].(string)
				if info := parseHelmRepo(name, commit, t.bitbucketURL); info != nil {
					repos = append(repos, *info)
				}
			}
		}
	}

	out := map[string]interface{}{
		"build_key":    buildKey,
		"build_state":  raw["buildState"],
		"triggered_by": raw["reasonSummary"],
		"repositories": repos,
	}
	if len(repos) == 0 {
		out["note"] = "No helm chart repositories found in this build (build may have been triggered without app code changes)"
	}

	content, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetPlanRepositories(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_plan_repositories args=%v", arguments)
	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	result, err := t.client.GetPlanRepositories(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get plan repositories: %v", err)), nil
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetBuildQueue(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_build_queue")
	queue, err := t.client.GetBuildQueue()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get build queue: %v", err)), nil
	}

	content, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListDeploymentProjects(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_deployment_projects args=%v", arguments)

	maxResult := 10
	if mr, ok := arguments["maxResult"].(float64); ok && mr > 0 {
		maxResult = int(mr)
	}

	projects, err := t.client.ListDeploymentProjects(maxResult)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list deployment projects: %v", err)), nil
	}

	result := map[string]interface{}{
		"returnedCount":      len(projects),
		"deploymentProjects": projects,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListDeploymentProjectsForPlan(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_deployment_projects_for_plan args=%v", arguments)

	planKey, ok := arguments["planKey"].(string)
	if !ok || planKey == "" {
		return mcp.NewToolResultError("planKey is required"), nil
	}

	projects, err := t.client.ListDeploymentProjectsForPlan(planKey)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list deployment projects for plan: %v", err)), nil
	}

	result := map[string]interface{}{
		"planKey":            planKey,
		"count":              len(projects),
		"deploymentProjects": projects,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetDeploymentProject(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_deployment_project args=%v", arguments)
	projectID, ok := arguments["projectId"].(string)
	if !ok || projectID == "" {
		return mcp.NewToolResultError("projectId is required"), nil
	}

	project, err := t.client.GetDeploymentProject(projectID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get deployment project: %v", err)), nil
	}

	content, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetDeploymentEnvironmentResults(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_environment_results args=%v", arguments)
	envID, ok := arguments["environmentId"].(string)
	if !ok || envID == "" {
		return mcp.NewToolResultError("environmentId is required"), nil
	}

	maxResults := 10
	if mr, ok := arguments["maxResults"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	results, err := t.client.GetDeploymentEnvironmentResults(envID, maxResults)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get environment results: %v", err)), nil
	}

	content, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetDeploymentResult(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_deployment_result args=%v", arguments)
	resultID, ok := arguments["resultId"].(string)
	if !ok || resultID == "" {
		return mcp.NewToolResultError("resultId is required"), nil
	}

	result, err := t.client.GetDeploymentResult(resultID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get deployment result: %v", err)), nil
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) ListDeploymentVersions(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_list_deploy_versions args=%v", arguments)
	projectID, ok := arguments["projectId"].(string)
	if !ok || projectID == "" {
		return mcp.NewToolResultError("projectId is required"), nil
	}

	maxResults := 10
	if mr, ok := arguments["maxResults"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	versions, err := t.client.ListDeploymentVersions(projectID, maxResults)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list deployment versions: %v", err)), nil
	}

	content, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetDeploymentVersion(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_deploy_version args=%v", arguments)
	versionID, ok := arguments["versionId"].(string)
	if !ok || versionID == "" {
		return mcp.NewToolResultError("versionId is required"), nil
	}

	version, err := t.client.GetDeploymentVersion(versionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get deployment version: %v", err)), nil
	}

	content, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BambooTools) GetDeploymentVersionStatus(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	logger.Info("Tool called: bamboo_get_deploy_version_status args=%v", arguments)
	versionID, ok := arguments["versionId"].(string)
	if !ok || versionID == "" {
		return mcp.NewToolResultError("versionId is required"), nil
	}

	statuses, err := t.client.GetDeploymentVersionStatus(versionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get version status: %v", err)), nil
	}

	result := map[string]interface{}{
		"versionId":           versionID,
		"environmentStatuses": statuses,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}
