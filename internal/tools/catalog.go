package tools

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// Handler is the signature shared by every tool implementation in this package.
// It matches server.ToolHandlerFunc, so a Handler can be registered as-is.
type Handler func(arguments map[string]interface{}) (*mcp.CallToolResult, error)

// Catalog returns the static declaration of every tool this server exposes.
//
// This is the single source of truth for the tool surface. cmd/server registers
// exactly these declarations, `bamboo-mcp --list-tools` prints them, tools.json
// in the repo root is generated from them, and catalog_test.go asserts that each
// one has a handler and is exercised by a test.
func Catalog() []mcp.Tool {
	catalog := make([]mcp.Tool, 0, 35)
	catalog = append(catalog, bambooCatalog()...)
	catalog = append(catalog, contextCatalog()...)
	catalog = append(catalog, bitbucketCatalog()...)
	return catalog
}

// CatalogJSON renders Catalog() as an MCP tools/list payload. The shape matches
// what a client receives over the wire, so it can be diffed against a live
// server.
func CatalogJSON() ([]byte, error) {
	return json.MarshalIndent(map[string]interface{}{"tools": Catalog()}, "", "  ")
}

// Handlers maps every tool name in Catalog() to its implementation. Registration
// fails loudly if the two ever drift apart.
func Handlers(b *BambooTools, c *ContextTools, bb *BitbucketTools) map[string]Handler {
	return map[string]Handler{
		// Server & projects
		"bamboo_server_info":       b.GetServerInfo,
		"bamboo_health_check":      b.HealthCheck,
		"bamboo_list_projects":     b.ListProjects,
		"bamboo_list_all_projects": b.ListAllProjects,
		"bamboo_get_project":       b.GetProject,

		// Plans & branches
		"bamboo_list_plans":         b.ListPlans,
		"bamboo_get_plan":           b.GetPlan,
		"bamboo_search_plans":       b.SearchPlans,
		"bamboo_list_plan_branches": b.ListPlanBranches,
		"bamboo_get_plan_branch":    b.GetPlanBranch,

		// Builds
		"bamboo_get_latest_result":         b.GetLatestResult,
		"bamboo_list_build_results":        b.ListBuildResults,
		"bamboo_get_build_result":          b.GetBuildResult,
		"bamboo_get_build_result_expanded": b.GetBuildResultExpanded,
		"bamboo_get_build_repositories":    b.GetBuildRepositories,
		"bamboo_get_plan_repositories":     b.GetPlanRepositories,
		"bamboo_get_build_queue":           b.GetBuildQueue,

		// Deployments
		"bamboo_list_deployment_projects":          b.ListDeploymentProjects,
		"bamboo_list_deployment_projects_for_plan": b.ListDeploymentProjectsForPlan,
		"bamboo_get_deployment_project":            b.GetDeploymentProject,
		"bamboo_get_environment_results":           b.GetDeploymentEnvironmentResults,
		"bamboo_get_deployment_result":             b.GetDeploymentResult,
		"bamboo_list_deploy_versions":              b.ListDeploymentVersions,
		"bamboo_get_deploy_version":                b.GetDeploymentVersion,
		"bamboo_get_deploy_version_status":         b.GetDeploymentVersionStatus,

		// Plan context
		"bamboo_get_plan_context":    c.GetPlanContext,
		"bamboo_reload_context":      c.ReloadContext,
		"bamboo_resolve_plan":        c.ResolvePlan,
		"bamboo_get_deploy_status":   c.GetDeployStatus,
		"bamboo_explain_environment": c.ExplainEnvironment,
		"bamboo_get_plan_type_info":  c.GetPlanTypeInfo,

		// Bitbucket credential storage
		"bitbucket_add_repo":    bb.AddRepo,
		"bitbucket_get_repo":    bb.GetRepo,
		"bitbucket_list_repos":  bb.ListRepos,
		"bitbucket_delete_repo": bb.DeleteRepo,
	}
}

// bambooCatalog declares the read-only tools that wrap the Bamboo REST API.
func bambooCatalog() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("bamboo_server_info",
			mcp.WithDescription("Get Bamboo server information"),
			mcp.WithString("expand", mcp.Description("Optional fields to expand")),
		),
		mcp.NewTool("bamboo_health_check",
			mcp.WithDescription("Check if Bamboo server is healthy and accessible"),
		),
		mcp.NewTool("bamboo_list_projects",
			mcp.WithDescription("List Bamboo projects with pagination"),
			mcp.WithNumber("maxResult", mcp.Description("Maximum number of results per page (default: 25)")),
			mcp.WithNumber("startIndex", mcp.Description("Start index for pagination (default: 0)")),
		),
		mcp.NewTool("bamboo_list_all_projects",
			mcp.WithDescription("List all Bamboo projects (fetches all pages automatically)"),
		),
		mcp.NewTool("bamboo_get_project",
			mcp.WithDescription("Get details of a specific Bamboo project"),
			mcp.WithString("projectKey", mcp.Required(), mcp.Description("The project key (e.g., 'PROJ')")),
		),
		mcp.NewTool("bamboo_list_plans",
			mcp.WithDescription("List all Bamboo build plans"),
		),
		mcp.NewTool("bamboo_get_plan",
			mcp.WithDescription("Get details of a specific Bamboo build plan"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		),
		mcp.NewTool("bamboo_search_plans",
			mcp.WithDescription("Search for Bamboo build plans"),
			mcp.WithString("searchTerm", mcp.Required(), mcp.Description("Search term to find plans")),
		),
		mcp.NewTool("bamboo_list_plan_branches",
			mcp.WithDescription("List all branches for a specific plan"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		),
		mcp.NewTool("bamboo_get_plan_branch",
			mcp.WithDescription("Get details of a specific plan branch"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
			mcp.WithString("branchName", mcp.Required(), mcp.Description("The branch name")),
		),
		mcp.NewTool("bamboo_get_latest_result",
			mcp.WithDescription("Get the latest build result for a plan"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		),
		mcp.NewTool("bamboo_list_build_results",
			mcp.WithDescription("List build results for a plan"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
			mcp.WithNumber("maxResults", mcp.Description("Maximum number of results to return (default: 25)")),
		),
		mcp.NewTool("bamboo_get_build_result",
			mcp.WithDescription("Get details of a specific build result"),
			mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build key (e.g., 'PROJ-PLAN-123')")),
		),
		mcp.NewTool("bamboo_get_build_result_expanded",
			mcp.WithDescription("Get expanded build result with changes, artifacts, metadata, comments, labels, JIRA issues, stages, and log URLs. Use this when you need detailed build information including what changed and deployment artifacts."),
			mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build key (e.g., 'PROJ-PLAN-123')")),
		),
		mcp.NewTool("bamboo_get_build_repositories",
			mcp.WithDescription("Get the repositories and commits (VCS revisions) associated with a specific build. Returns repository names, commit IDs, authors, and branch info. Use this to find which repo/commit was released in a build."),
			mcp.WithString("buildKey", mcp.Required(), mcp.Description("The build result key (e.g., 'EXAMPLE-APPCHECKOUT-4859')")),
		),
		mcp.NewTool("bamboo_get_plan_repositories",
			mcp.WithDescription("Get the repositories linked to a Bamboo plan (VCS locations). Shows all source repositories configured for the plan. Use this to see which repos a plan tracks."),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'EXAMPLE-APPCHECKOUT')")),
		),
		mcp.NewTool("bamboo_get_build_queue",
			mcp.WithDescription("Get the current build queue"),
		),
		mcp.NewTool("bamboo_list_deployment_projects",
			mcp.WithDescription("List deployment projects (default: 10, truncated client-side from full list)"),
			mcp.WithNumber("maxResult", mcp.Description("Maximum number of deployment projects to return (default: 10)")),
		),
		mcp.NewTool("bamboo_list_deployment_projects_for_plan",
			mcp.WithDescription("List deployment projects linked to a specific build plan"),
			mcp.WithString("planKey", mcp.Required(), mcp.Description("The plan key (e.g., 'PROJ-PLAN')")),
		),
		mcp.NewTool("bamboo_get_deployment_project",
			mcp.WithDescription("Get details of a specific deployment project"),
			mcp.WithString("projectId", mcp.Required(), mcp.Description("The deployment project ID")),
		),
		mcp.NewTool("bamboo_get_environment_results",
			mcp.WithDescription("Get deployment history/results for a specific environment. Returns who deployed, when, status, version, and deployment result IDs."),
			mcp.WithString("environmentId", mcp.Required(), mcp.Description("The deployment environment ID")),
			mcp.WithNumber("maxResults", mcp.Description("Maximum number of results to return (default: 10)")),
		),
		mcp.NewTool("bamboo_get_deployment_result",
			mcp.WithDescription("Get full details of a specific deployment result including who triggered it, start/finish time, status, deployment version, and agent info."),
			mcp.WithString("resultId", mcp.Required(), mcp.Description("The deployment result ID")),
		),
		mcp.NewTool("bamboo_list_deploy_versions",
			mcp.WithDescription("List versions (releases) for a deployment project. Shows release names, creation dates, and associated plan branch."),
			mcp.WithString("projectId", mcp.Required(), mcp.Description("The deployment project ID")),
			mcp.WithNumber("maxResults", mcp.Description("Maximum number of versions to return (default: 10)")),
		),
		mcp.NewTool("bamboo_get_deploy_version",
			mcp.WithDescription("Get details of a specific deployment version/release including name, creation date, creator, plan branch, and items."),
			mcp.WithString("versionId", mcp.Required(), mcp.Description("The deployment version/release ID")),
		),
		mcp.NewTool("bamboo_get_deploy_version_status",
			mcp.WithDescription("Get the deployment status of a specific version across all environments. Shows which environments have this version deployed and their status."),
			mcp.WithString("versionId", mcp.Required(), mcp.Description("The deployment version/release ID")),
		),
	}
}

// contextCatalog declares the tools that read the plan-context YAML to translate
// between human references and Bamboo keys.
func contextCatalog() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("bamboo_get_plan_context",
			mcp.WithDescription("Get the plan context knowledge configuration. Returns naming conventions for plans, deployment environments, modules, and actions. Use this to understand how plans and deployments are structured."),
		),
		mcp.NewTool("bamboo_reload_context",
			mcp.WithDescription("Reload the plan context configuration from the context.yaml file. Use after editing the config."),
		),
		mcp.NewTool("bamboo_resolve_plan",
			mcp.WithDescription("Resolve a human-friendly reference name to a full Bamboo plan key. For example: reference='CHECKOUT' with planType='app' resolves to 'EXAMPLE-CHECKOUTAPP'. Searches Bamboo if direct lookup fails."),
			mcp.WithString("reference", mcp.Required(), mcp.Description("The team/project reference name (e.g., 'CHECKOUT', 'PAYMENTS', 'APIGW')")),
			mcp.WithString("planType", mcp.Description("Plan type: 'app', 'infra'. If omitted, tries direct key lookup.")),
		),
		mcp.NewTool("bamboo_get_deploy_status",
			mcp.WithDescription("Smart deployment status checker. Resolves a reference to a plan, gets latest build status, finds deployment projects, filters environments, and fetches per-environment deployment history (who, when, status, version). Returns build status, deployment environments with latest deployment details, and Bamboo UI links."),
			mcp.WithString("reference", mcp.Required(), mcp.Description("The team/project reference name (e.g., 'CHECKOUT', 'PAYMENTS')")),
			mcp.WithString("planType", mcp.Description("Plan type: 'app', 'infra'. If omitted, tries direct key lookup.")),
			mcp.WithString("environment", mcp.Description("Filter environments by name (e.g., 'dev', 'staging', 'prod')")),
			mcp.WithString("module", mcp.Description("Filter environments by module (e.g., 'network', 'database', 'storage')")),
		),
		mcp.NewTool("bamboo_explain_environment",
			mcp.WithDescription("Parse a deployment environment name into its components (env, module, ref, action) using plan context knowledge. When the plan type declares modules, it resolves the module name and description. Provide planType for accurate parsing."),
			mcp.WithString("environmentName", mcp.Required(), mcp.Description("The environment name to parse (e.g., 'staging_network_checkout_deploy', 'dev_deploy', 'prod_nodegroup_inspect')")),
			mcp.WithString("planType", mcp.Description("Plan type for accurate parsing: 'app', 'infra'")),
		),
		mcp.NewTool("bamboo_get_plan_type_info",
			mcp.WithDescription("Get detailed information about a specific plan type including its naming format, environment format, available actions with descriptions, modules, log patterns for success detection, custom inputs, and hints."),
			mcp.WithString("planType", mcp.Required(), mcp.Description("Plan type: 'app', 'infra'")),
		),
	}
}

// bitbucketCatalog declares the optional Bitbucket credential-storage tools.
func bitbucketCatalog() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("bitbucket_add_repo",
			mcp.WithDescription("Add a Bitbucket repository configuration"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias for the repository")),
			mcp.WithString("url", mcp.Required(), mcp.Description("Bitbucket repository URL")),
			mcp.WithString("username", mcp.Required(), mcp.Description("Bitbucket username")),
			mcp.WithString("token", mcp.Required(), mcp.Description("Bitbucket access token")),
		),
		mcp.NewTool("bitbucket_get_repo",
			mcp.WithDescription("Get a stored Bitbucket repository configuration"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias of the repository")),
		),
		mcp.NewTool("bitbucket_list_repos",
			mcp.WithDescription("List all stored Bitbucket repository configurations"),
		),
		mcp.NewTool("bitbucket_delete_repo",
			mcp.WithDescription("Delete a stored Bitbucket repository configuration"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Name/alias of the repository to delete")),
		),
	}
}
