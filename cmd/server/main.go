package main

import (
	"log"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	"github.com/hmdmph/bamboo-mcp/internal/config"
	pctx "github.com/hmdmph/bamboo-mcp/internal/context"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/hmdmph/bamboo-mcp/internal/prompts"
	"github.com/hmdmph/bamboo-mcp/internal/security"
	"github.com/hmdmph/bamboo-mcp/internal/storage"
	"github.com/hmdmph/bamboo-mcp/internal/tools"
	"github.com/hmdmph/bamboo-mcp/internal/validation"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logger.SetVerbose(cfg.Verbose)
	logger.Info("Starting Bamboo MCP Server v1.4.0")
	logger.Info("Bamboo URL: %s", cfg.BambooURL)
	logger.Info("Transport: %s", cfg.Transport)

	bambooClient, err := bamboo.NewClient(cfg.BambooURL, cfg.BambooToken, cfg.BambooProxy)
	if err != nil {
		log.Fatalf("Failed to create Bamboo client: %v", err)
	}

	// Startup validation
	if _, err := validation.Run(cfg, bambooClient); err != nil {
		log.Fatalf("Startup validation failed: %v", err)
	}

	// Security layer
	secCfg, err := security.LoadSecurityConfig(cfg.SecurityFilePath)
	if err != nil {
		log.Fatalf("Failed to load security config: %v", err)
	}
	policy := security.NewPolicyEngine(secCfg)
	sanitizer := security.NewSanitizer(secCfg)
	logger.Info("Security: policy engine ready (sanitize=%v, rate_limit=%d/min)",
		secCfg.SanitizeResponses, secCfg.DefaultRateLimitPerMinute)

	bitbucketStorage, err := storage.NewBitbucketStorage()
	if err != nil {
		log.Fatalf("Failed to create Bitbucket storage: %v", err)
	}

	planContext, err := pctx.NewPlanContext(cfg.ContextFilePath)
	if err != nil {
		log.Fatalf("Failed to load plan context: %v", err)
	}

	bambooTools := tools.NewBambooTools(bambooClient, cfg.BitbucketURL)
	bitbucketTools := tools.NewBitbucketTools(bitbucketStorage)
	contextTools := tools.NewContextTools(bambooClient, planContext)

	s := server.NewMCPServer(
		"Bamboo MCP Server",
		"1.4.0",
		server.WithPromptCapabilities(false),
	)

	registerBambooTools(s, bambooTools, policy, sanitizer)
	registerBitbucketTools(s, bitbucketTools, policy, sanitizer)
	registerContextTools(s, contextTools, policy, sanitizer)
	prompts.Register(s)

	switch cfg.Transport {
	case config.TransportSSE:
		addr := cfg.HTTPAddr()
		baseURL := cfg.BaseURL()
		logger.Info("Starting SSE/HTTP server on %s (base URL: %s)", addr, baseURL)
		sseServer := server.NewSSEServer(s, baseURL)
		if err := sseServer.Start(addr); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}
	default:
		logger.Info("Starting stdio server")
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

// w is a shorthand for policy.Wrap to keep registration lines concise.
func w(name, method string, h server.ToolHandlerFunc, p *security.PolicyEngine, san *security.Sanitizer) server.ToolHandlerFunc {
	return p.Wrap(name, method, h, san)
}

func registerBambooTools(s *server.MCPServer, t *tools.BambooTools, p *security.PolicyEngine, san *security.Sanitizer) {
	s.AddTool(mcp.NewTool("bamboo_server_info",
		mcp.WithDescription("Get Bamboo server information"),
		mcp.WithString("expand", mcp.Description("Optional fields to expand")),
	), w("bamboo_server_info", "GET", t.GetServerInfo, p, san))

	s.AddTool(mcp.NewTool("bamboo_health_check",
		mcp.WithDescription("Check if Bamboo server is healthy and accessible"),
	), w("bamboo_health_check", "GET", t.HealthCheck, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_projects",
		mcp.WithDescription("List Bamboo projects with pagination"),
		mcp.WithNumber("maxResult", mcp.Description("Maximum number of results per page (default: 25)")),
		mcp.WithNumber("startIndex", mcp.Description("Start index for pagination (default: 0)")),
	), w("bamboo_list_projects", "GET", t.ListProjects, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_all_projects",
		mcp.WithDescription("List all Bamboo projects (fetches all pages automatically)"),
	), w("bamboo_list_all_projects", "GET", t.ListAllProjects, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_project",
		mcp.WithDescription("Get details of a specific Bamboo project"),
		mcp.WithString("projectKey", mcp.Required(), mcp.Description("The project key (e.g., 'PROJ')")),
	), w("bamboo_get_project", "GET", t.GetProject, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_plans",
		mcp.WithDescription("List all Bamboo build plans"),
	), w("bamboo_list_plans", "GET", t.ListPlans, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_plan",
		mcp.WithDescription("Get details of a specific Bamboo build plan"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
	), w("bamboo_get_plan", "GET", t.GetPlan, p, san))

	s.AddTool(mcp.NewTool("bamboo_search_plans",
		mcp.WithDescription("Search for Bamboo build plans"),
		mcp.WithString("searchTerm", mcp.Required(), mcp.Description("Search term to find plans")),
	), w("bamboo_search_plans", "GET", t.SearchPlans, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_plan_branches",
		mcp.WithDescription("List all branches for a specific plan"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
	), w("bamboo_list_plan_branches", "GET", t.ListPlanBranches, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_plan_branch",
		mcp.WithDescription("Get details of a specific plan branch"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		mcp.WithString("branchName", mcp.Required(), mcp.Description("The branch name")),
	), w("bamboo_get_plan_branch", "GET", t.GetPlanBranch, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_latest_result",
		mcp.WithDescription("Get the latest build result for a plan"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
	), w("bamboo_get_latest_result", "GET", t.GetLatestResult, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_build_results",
		mcp.WithDescription("List build results for a plan"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results to return (default: 25)")),
	), w("bamboo_list_build_results", "GET", t.ListBuildResults, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_build_result",
		mcp.WithDescription("Get details of a specific build result"),
		mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build key (e.g., 'PROJ-PLAN-123')")),
	), w("bamboo_get_build_result", "GET", t.GetBuildResult, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_build_result_expanded",
		mcp.WithDescription("Get expanded build result with changes, artifacts, metadata, comments, labels, JIRA issues, stages, and log URLs. Use this when you need detailed build information including what changed and deployment artifacts."),
		mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build key (e.g., 'PROJ-PLAN-123')")),
	), w("bamboo_get_build_result_expanded", "GET", t.GetBuildResultExpanded, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_build_repositories",
		mcp.WithDescription("Get the repositories and commits (VCS revisions) associated with a specific build. Returns repository names, commit IDs, authors, and branch info. Use this to find which repo/commit was released in a build."),
		mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build result key (e.g., 'EXAMPLE-APPCHECKOUT-4859')")),
	), w("bamboo_get_build_repositories", "GET", t.GetBuildRepositories, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_plan_repositories",
		mcp.WithDescription("Get the repositories linked to a Bamboo plan (VCS locations). Shows all source repositories configured for the plan. Use this to see which repos a plan tracks."),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'EXAMPLE-APPCHECKOUT')")),
	), w("bamboo_get_plan_repositories", "GET", t.GetPlanRepositories, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_build_queue",
		mcp.WithDescription("Get the current build queue"),
	), w("bamboo_get_build_queue", "GET", t.GetBuildQueue, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_deployment_projects",
		mcp.WithDescription("List deployment projects (default: 10, truncated client-side from full list)"),
		mcp.WithNumber("maxResult", mcp.Description("Maximum number of deployment projects to return (default: 10)")),
	), w("bamboo_list_deployment_projects", "GET", t.ListDeploymentProjects, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_deployment_projects_for_plan",
		mcp.WithDescription("List deployment projects linked to a specific build plan"),
		mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
	), w("bamboo_list_deployment_projects_for_plan", "GET", t.ListDeploymentProjectsForPlan, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_deployment_project",
		mcp.WithDescription("Get details of a specific deployment project"),
		mcp.WithString("projectId", mcp.Required(), mcp.Description("The deployment project ID")),
	), w("bamboo_get_deployment_project", "GET", t.GetDeploymentProject, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_environment_results",
		mcp.WithDescription("Get deployment history/results for a specific environment. Returns who deployed, when, status, version, and deployment result IDs."),
		mcp.WithString("environmentId", mcp.Required(), mcp.Description("The deployment environment ID")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results to return (default: 10)")),
	), w("bamboo_get_environment_results", "GET", t.GetDeploymentEnvironmentResults, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_deployment_result",
		mcp.WithDescription("Get full details of a specific deployment result including who triggered it, start/finish time, status, deployment version, and agent info."),
		mcp.WithString("resultId", mcp.Required(), mcp.Description("The deployment result ID")),
	), w("bamboo_get_deployment_result", "GET", t.GetDeploymentResult, p, san))

	s.AddTool(mcp.NewTool("bamboo_list_deploy_versions",
		mcp.WithDescription("List versions (releases) for a deployment project. Shows release names, creation dates, and associated plan branch."),
		mcp.WithString("projectId", mcp.Required(), mcp.Description("The deployment project ID")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of versions to return (default: 10)")),
	), w("bamboo_list_deploy_versions", "GET", t.ListDeploymentVersions, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_deploy_version",
		mcp.WithDescription("Get details of a specific deployment version/release including name, creation date, creator, plan branch, and items."),
		mcp.WithString("versionId", mcp.Required(), mcp.Description("The deployment version/release ID")),
	), w("bamboo_get_deploy_version", "GET", t.GetDeploymentVersion, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_deploy_version_status",
		mcp.WithDescription("Get the deployment status of a specific version across all environments. Shows which environments have this version deployed and their status."),
		mcp.WithString("versionId", mcp.Required(), mcp.Description("The deployment version/release ID")),
	), w("bamboo_get_deploy_version_status", "GET", t.GetDeploymentVersionStatus, p, san))
}

func registerContextTools(s *server.MCPServer, t *tools.ContextTools, p *security.PolicyEngine, san *security.Sanitizer) {
	s.AddTool(mcp.NewTool("bamboo_get_plan_context",
		mcp.WithDescription("Get the plan context knowledge configuration. Returns naming conventions for plans, deployment environments, modules, and actions. Use this to understand how plans and deployments are structured."),
	), w("bamboo_get_plan_context", "GET", t.GetPlanContext, p, san))

	s.AddTool(mcp.NewTool("bamboo_reload_context",
		mcp.WithDescription("Reload the plan context configuration from the context.yaml file. Use after editing the config."),
	), w("bamboo_reload_context", "GET", t.ReloadContext, p, san))

	s.AddTool(mcp.NewTool("bamboo_resolve_plan",
		mcp.WithDescription("Resolve a human-friendly reference name to a full Bamboo plan key. For example: reference='CHECKOUT' with planType='app' resolves to 'EXAMPLE-CHECKOUTAPP'. Searches Bamboo if direct lookup fails."),
		mcp.WithString("reference", mcp.Required(), mcp.Description("The team/project reference name (e.g., 'CHECKOUT', 'PAYMENTS', 'APIGW')")),
		mcp.WithString("planType", mcp.Description("Plan type: 'app', 'infra'. If omitted, tries direct key lookup.")),
	), w("bamboo_resolve_plan", "GET", t.ResolvePlan, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_deploy_status",
		mcp.WithDescription("Smart deployment status checker. Resolves a reference to a plan, gets latest build status, finds deployment projects, filters environments, and fetches per-environment deployment history (who, when, status, version). Returns build status, deployment environments with latest deployment details, and Bamboo UI links."),
		mcp.WithString("reference", mcp.Required(), mcp.Description("The team/project reference name (e.g., 'CHECKOUT', 'PAYMENTS')")),
		mcp.WithString("planType", mcp.Description("Plan type: 'app', 'infra'. If omitted, tries direct key lookup.")),
		mcp.WithString("environment", mcp.Description("Filter environments by name (e.g., 'dev', 'staging', 'prod')")),
		mcp.WithString("module", mcp.Description("Filter environments by module (e.g., 'network', 'database', 'storage')")),
	), w("bamboo_get_deploy_status", "GET", t.GetDeployStatus, p, san))

	s.AddTool(mcp.NewTool("bamboo_explain_environment",
		mcp.WithDescription("Parse a deployment environment name into its components (env, module, ref, action) using plan context knowledge. When the plan type declares modules, it resolves the module name and description. Provide planType for accurate parsing."),
		mcp.WithString("environmentName", mcp.Required(), mcp.Description("The environment name to parse (e.g., 'staging_network_checkout_deploy', 'dev_deploy', 'prod_nodegroup_inspect')")),
		mcp.WithString("planType", mcp.Description("Plan type for accurate parsing: 'app', 'infra'")),
	), w("bamboo_explain_environment", "GET", t.ExplainEnvironment, p, san))

	s.AddTool(mcp.NewTool("bamboo_get_plan_type_info",
		mcp.WithDescription("Get detailed information about a specific plan type including its naming format, environment format, available actions with descriptions, modules, log patterns for success detection, custom inputs, and hints."),
		mcp.WithString("planType", mcp.Required(), mcp.Description("Plan type: 'app', 'infra'")),
	), w("bamboo_get_plan_type_info", "GET", t.GetPlanTypeInfo, p, san))
}

func registerBitbucketTools(s *server.MCPServer, t *tools.BitbucketTools, p *security.PolicyEngine, san *security.Sanitizer) {
	s.AddTool(mcp.NewTool("bitbucket_add_repo",
		mcp.WithDescription("Add a Bitbucket repository configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias for the repository")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Bitbucket repository URL")),
		mcp.WithString("username", mcp.Required(), mcp.Description("Bitbucket username")),
		mcp.WithString("token", mcp.Required(), mcp.Description("Bitbucket access token")),
	), w("bitbucket_add_repo", "GET", t.AddRepo, p, san))

	s.AddTool(mcp.NewTool("bitbucket_get_repo",
		mcp.WithDescription("Get a stored Bitbucket repository configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias of the repository")),
	), w("bitbucket_get_repo", "GET", t.GetRepo, p, san))

	s.AddTool(mcp.NewTool("bitbucket_list_repos",
		mcp.WithDescription("List all stored Bitbucket repository configurations"),
	), w("bitbucket_list_repos", "GET", t.ListRepos, p, san))

	s.AddTool(mcp.NewTool("bitbucket_delete_repo",
		mcp.WithDescription("Delete a stored Bitbucket repository configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias of the repository to delete")),
	), w("bitbucket_delete_repo", "GET", t.DeleteRepo, p, san))
}
