package prompts

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Register registers all MCP prompt templates onto the server.
func Register(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("deploy_status",
		mcp.WithPromptDescription("Check the deployment status of a team/project across environments. Resolves the plan, shows latest build, and lists per-environment deployment history (who, when, version)."),
		mcp.WithArgument("reference", mcp.ArgumentDescription("Team or project reference (e.g. CHECKOUT, PAYMENTS). Remove hyphens and uppercase: pay-ments-api → PAYMENTS."), mcp.RequiredArgument()),
		mcp.WithArgument("plan_type", mcp.ArgumentDescription("Plan type: app, infra")),
		mcp.WithArgument("environment", mcp.ArgumentDescription("Filter by environment name (e.g. dev, staging, prod). REQUIRED — never infer from context.")),
		mcp.WithArgument("module", mcp.ArgumentDescription("Filter by module (e.g. network, database, storage)")),
	), handleDeployStatus)

	s.AddPrompt(mcp.NewPrompt("who_deployed_last",
		mcp.WithPromptDescription("Find who ran the latest deployment for a specific project, module, and environment. Handles reference resolution, fallback search, and environment ID lookup automatically."),
		mcp.WithArgument("reference", mcp.ArgumentDescription("Team or project reference — remove hyphens and uppercase (e.g. pay-ments-api → PAYMENTS)"), mcp.RequiredArgument()),
		mcp.WithArgument("module", mcp.ArgumentDescription("Module name (e.g. network, database, storage)")),
		mcp.WithArgument("environment", mcp.ArgumentDescription("Environment to check (dev, staging, prod)"), mcp.RequiredArgument()),
		mcp.WithArgument("plan_type", mcp.ArgumentDescription("Plan type: app, infra (default: app)")),
	), handleWhoDeployedLast)

	s.AddPrompt(mcp.NewPrompt("explain_environment",
		mcp.WithPromptDescription("Explain what a deployment environment name means — parses it into env, module, ref, and action components with descriptions."),
		mcp.WithArgument("environment_name", mcp.ArgumentDescription("The environment name to explain (e.g. dev_network_payments_deploy, staging_network_checkout_deploy)"), mcp.RequiredArgument()),
		mcp.WithArgument("plan_type", mcp.ArgumentDescription("Plan type for accurate parsing: app, infra")),
	), handleExplainEnvironment)

	s.AddPrompt(mcp.NewPrompt("plan_type_guide",
		mcp.WithPromptDescription("Get a complete guide for a plan type: naming format, environment format, all actions with descriptions, modules, log patterns for success detection, custom inputs, and usage hints."),
		mcp.WithArgument("plan_type", mcp.ArgumentDescription("Plan type: app, infra"), mcp.RequiredArgument()),
	), handlePlanTypeGuide)

	s.AddPrompt(mcp.NewPrompt("build_investigation",
		mcp.WithPromptDescription("Investigate a specific build result — get expanded details including changes, artifacts, stages, and a direct link to the build log."),
		mcp.WithArgument("build_key", mcp.ArgumentDescription("The build result key (e.g. EXAMPLE-PAYMENTSINFRA-430, EXAMPLE-APPCHECKOUT-541)"), mcp.RequiredArgument()),
	), handleBuildInvestigation)

	s.AddPrompt(mcp.NewPrompt("deployment_history",
		mcp.WithPromptDescription("Show who deployed what and when for a specific deployment environment. Returns the last N deployments with deployer, timestamp, version, and status."),
		mcp.WithArgument("environment_id", mcp.ArgumentDescription("The deployment environment ID (numeric, from bamboo_get_deployment_project)"), mcp.RequiredArgument()),
		mcp.WithArgument("limit", mcp.ArgumentDescription("Number of recent deployments to show (default: 10)")),
	), handleDeploymentHistory)

	s.AddPrompt(mcp.NewPrompt("resolve_plan",
		mcp.WithPromptDescription("Resolve a human-friendly team/project reference to a full Bamboo plan key and show its latest build status."),
		mcp.WithArgument("reference", mcp.ArgumentDescription("Team or project reference — remove hyphens and uppercase (e.g. CHECKOUT, PAYMENTS)"), mcp.RequiredArgument()),
		mcp.WithArgument("plan_type", mcp.ArgumentDescription("Plan type: app, infra")),
	), handleResolvePlan)

	s.AddPrompt(mcp.NewPrompt("build_repositories",
		mcp.WithPromptDescription("Find which repositories and commits were included in a specific build or plan. Use when asked 'which repo/commit was released', 'what code changes are in this build', or 'which repos does this plan track'."),
		mcp.WithArgument("build_key", mcp.ArgumentDescription("Build result key to get commit/repo info for (e.g. EXAMPLE-APPCHECKOUT-4859). Use this for a specific build.")),
		mcp.WithArgument("plan_key", mcp.ArgumentDescription("Plan key to get all tracked repositories (e.g. EXAMPLE-APPCHECKOUT). Use this for repo config of the plan.")),
	), handleBuildRepositories)

	s.AddPrompt(mcp.NewPrompt("health_check",
		mcp.WithPromptDescription("Run a full health check of the Bamboo server — connectivity, authentication, server version, and build queue status."),
	), handleHealthCheck)
}

func handleDeployStatus(args map[string]string) (*mcp.GetPromptResult, error) {
	ref := args["reference"]
	planType := args["plan_type"]
	env := args["environment"]
	module := args["module"]

	instructions := "## Reference Sanitisation (do this first)\n"
	instructions += "- Remove hyphens and uppercase: \"pay-ments-api\" → \"PAYMENTS\", \"pay-ments-api\" → \"PAYMENTS\"\n"
	instructions += "- If the reference already ends with the plan type suffix (e.g. ends in \"Infra\"), do NOT append it again\n\n"
	instructions += fmt.Sprintf("## Step 1: Call bamboo_get_deploy_status\n- reference=\"%s\"\n", ref)
	if planType != "" {
		instructions += fmt.Sprintf("- planType=\"%s\"\n", planType)
	}
	if env != "" {
		instructions += fmt.Sprintf("- environment=\"%s\" (always pass explicitly — never infer)\n", env)
	} else {
		instructions += "- environment=<must be explicitly provided: dev, staging, or prod>\n"
	}
	if module != "" {
		instructions += fmt.Sprintf("- module=\"%s\"\n", module)
	}
	instructions += `
## Step 2: If plan not found (404 error)
1. Call bamboo_search_plans with the reference as searchTerm
2. Identify the correct plan key (planName containing the reference + plan type)
3. Call bamboo_list_deployment_projects_for_plan with that plan key
4. Call bamboo_get_deployment_project to list all environments
5. Filter environments matching the requested env/module, then call bamboo_get_environment_results

## Step 3: Present results
1. Latest build: state, build number, duration, triggered by
2. Per environment: name, deployer (who), date (when), version, status
3. Bamboo UI links for plan and latest build
4. Note any environments with no deployment history`

	return promptResult("Deployment Status: "+ref, instructions), nil
}

func handleWhoDeployedLast(args map[string]string) (*mcp.GetPromptResult, error) {
	ref := args["reference"]
	module := args["module"]
	env := args["environment"]
	planType := args["plan_type"]
	if planType == "" {
		planType = "app"
	}

	moduleFilter := ""
	if module != "" {
		moduleFilter = fmt.Sprintf(", module=\"%s\"", module)
	}

	instructions := fmt.Sprintf(`## Goal
Find who ran the latest "%s" deployment for "%s" in the "%s" environment.

## Reference Sanitisation
- Remove hyphens and uppercase: "pay-ments-api" → "PAYMENTS"
- Do NOT double-append plan type suffix if reference already contains it
  (e.g. "PAYMENTSINFRA" is already the full plan key — pass it directly to bamboo_list_deployment_projects_for_plan)

## Fast path — try bamboo_get_deploy_status first:
  reference="%s", planType="%s", environment="%s"%s

If that succeeds, read latest_deployment from the matched environment and report:
- **Who**: triggered_by (extract the display name)
- **When**: executed_date (human-readable)
- **Status**: deploymentState
- **Version**: deploymentVersionName
- **Log**: log_url

## Fallback if plan not found (404):
1. Call bamboo_search_plans with searchTerm="%s" to find the correct plan key
2. Call bamboo_list_deployment_projects_for_plan with the resolved plan key
3. Call bamboo_get_deployment_project with the deployment project ID
4. Scan environments for names matching pattern: {env}_{module}_{ref}_deploy
   Example: "dev_network_payments_deploy"
5. Call bamboo_get_environment_results with that environment ID and maxResults=1
6. Report who deployed, when, version, and status`,
		module, ref, env,
		ref, planType, env, moduleFilter,
		ref)

	return promptResult(fmt.Sprintf("Who Deployed Last: %s / %s / %s", ref, module, env), instructions), nil
}

func handleExplainEnvironment(args map[string]string) (*mcp.GetPromptResult, error) {
	envName := args["environment_name"]
	planType := args["plan_type"]

	instructions := fmt.Sprintf(
		`Use bamboo_explain_environment with environmentName="%s"`, envName)
	if planType != "" {
		instructions += fmt.Sprintf(` and planType="%s"`, planType)
	}
	instructions += `.

Present the parsed result as:
- Environment name: (raw value)
- Components: list each component (environment, module, ref, action) with its value and description
- Module description (when the plan type declares modules): what this module manages
- Action description: what this action does
- Plan type format: the expected naming pattern`

	return promptResult("Explain Environment: "+envName, instructions), nil
}

func handlePlanTypeGuide(args map[string]string) (*mcp.GetPromptResult, error) {
	planType := args["plan_type"]

	instructions := fmt.Sprintf(
		`Use bamboo_get_plan_type_info with planType="%s".

Present a comprehensive guide structured as:
## %s Plan Type Guide
- **Description**: what this plan type does
- **Plan naming format**: how plans are named
- **Environment format**: how deployment environments are named
- **Available environments**: list of standard environments
- **Available actions**: table with action name and description
`, planType, planType)

	instructions += `- **Modules**: if the plan type defines modules, list each with its description
- **Custom inputs**: any custom variables the plan accepts, with their possible values
- **Log patterns**: any patterns that indicate a successful or failed run
- **Hints**: domain-specific tips for working with this plan type

Omit any section the plan type does not define.`

	return promptResult("Plan Type Guide: "+planType, instructions), nil
}

func handleBuildInvestigation(args map[string]string) (*mcp.GetPromptResult, error) {
	buildKey := args["build_key"]

	instructions := fmt.Sprintf(
		`Use bamboo_get_build_result_expanded with buildKey="%s".

Present the investigation as:
1. **Build summary**: key, state, build number, duration, reason for build
2. **Changes**: list of commits/changesets that triggered this build
3. **Artifacts**: any produced artifacts
4. **Stages**: stage names and their results
5. **Log access**: direct link to view build logs in Bamboo UI
6. **JIRA issues**: any linked JIRA tickets`, buildKey)

	return promptResult("Build Investigation: "+buildKey, instructions), nil
}

func handleDeploymentHistory(args map[string]string) (*mcp.GetPromptResult, error) {
	envID := args["environment_id"]
	limit := args["limit"]
	if limit == "" {
		limit = "10"
	}

	instructions := fmt.Sprintf(
		`Use bamboo_get_environment_results with environmentId="%s" and maxResults=%s.

Present the deployment history as a table with columns:
| # | Version | Deployer | Date/Time | Status | Duration |

Then summarise:
- Most recent successful deployment
- Any failed deployments and when they occurred
- Deployment frequency pattern`, envID, limit)

	return promptResult("Deployment History: env "+envID, instructions), nil
}

func handleResolvePlan(args map[string]string) (*mcp.GetPromptResult, error) {
	ref := args["reference"]
	planType := args["plan_type"]

	instructions := fmt.Sprintf(
		`Step 1: Use bamboo_resolve_plan with reference="%s"`, ref)
	if planType != "" {
		instructions += fmt.Sprintf(` and planType="%s"`, planType)
	}
	instructions += `.
Step 2: Once you have the plan key, use bamboo_get_latest_result to get the latest build status.

Present:
- Resolved plan key
- Plan name
- Latest build: state, number, completion time, duration
- Direct link to the plan in Bamboo UI`

	return promptResult("Resolve Plan: "+ref, instructions), nil
}

func handleBuildRepositories(args map[string]string) (*mcp.GetPromptResult, error) {
	buildKey := args["build_key"]
	planKey := args["plan_key"]

	var instructions string

	if buildKey != "" && planKey != "" {
		instructions = fmt.Sprintf(`## Goal
Show repositories and commits for build "%s" AND all repos tracked by plan "%s".

## Step 1 — Build-specific commits
Call bamboo_get_build_repositories with buildKey="%s".
Extract from vcsRevisions: repository name, commit ID, author, branch, commit message.

## Step 2 — Plan-level repositories
Call bamboo_get_plan_repositories with planKey="%s".
Extract from vcsLocations: repository name, URL, branch/default branch.

## Present as:
### Repositories in Build %s
| Repository | Branch | Commit ID | Author |
|------------|--------|-----------|--------|

### All Repositories Tracked by Plan
| Repository | URL | Branch |
|------------|-----|--------|`, buildKey, planKey, buildKey, planKey, buildKey)

	} else if buildKey != "" {
		instructions = fmt.Sprintf(`## Goal
Show which repositories and commits were included in build "%s".

## Step 1
Call bamboo_get_build_repositories with buildKey="%s".

## Step 2 — If you also need the plan's full repo list
Extract the planKey from the build result, then call bamboo_get_plan_repositories with that planKey.

## Present as:
### Commits in Build %s
| Repository | Branch | Commit ID | Author | Message |
|------------|--------|-----------|--------|---------|

- If vcsRevisions is empty, note that this build had no code changes (e.g. manual trigger).
- Show the vcsRevisionKey (overall Git revision) as a summary.`, buildKey, buildKey, buildKey)

	} else if planKey != "" {
		instructions = fmt.Sprintf(`## Goal
Show all repositories tracked by plan "%s".

## Step 1
Call bamboo_get_plan_repositories with planKey="%s".

## Step 2 — To see the latest commit per repo
Call bamboo_get_latest_result with planKey="%s", then call bamboo_get_build_repositories with the returned buildKey.

## Present as:
### Repositories Configured for Plan %s
| Repository | URL | Branch |
|------------|-----|--------|

### Latest Build Commits (from most recent build)
| Repository | Commit ID | Author | Branch |
|------------|-----------|--------|--------|`, planKey, planKey, planKey, planKey)

	} else {
		instructions = `## Usage
Provide at least one of:
- build_key: to see commits/repos in a specific build (e.g. EXAMPLE-APPCHECKOUT-4859)
- plan_key: to see all repos tracked by a plan (e.g. EXAMPLE-APPCHECKOUT)

## Tools to use:
- bamboo_get_build_repositories(buildKey) — commits + repos for a build
- bamboo_get_plan_repositories(planKey) — all VCS repos configured for a plan

## Tip
If you only know the project reference (e.g. CHECKOUT), first resolve it:
1. bamboo_search_plans with searchTerm to get the plan key
2. bamboo_get_latest_result to get the latest buildKey
3. Then call bamboo_get_build_repositories`
	}

	title := "Build Repositories"
	if buildKey != "" {
		title = "Build Repositories: " + buildKey
	} else if planKey != "" {
		title = "Plan Repositories: " + planKey
	}

	return promptResult(title, instructions), nil
}

func handleHealthCheck(_ map[string]string) (*mcp.GetPromptResult, error) {
	instructions := `Run the following checks in order and present a health report:

1. Use bamboo_health_check — confirm Bamboo is reachable
2. Use bamboo_server_info — get version and build info
3. Use bamboo_get_build_queue — check if there are pending/running builds

Present results as:
## Bamboo Server Health Report
- **Connectivity**: ✓/✗ with details
- **Server version**: version string
- **Build info**: any relevant server metadata
- **Build queue**: number of items in queue, list them if any
- **Overall status**: Healthy / Degraded / Unreachable`

	return promptResult("Bamboo Health Check", instructions), nil
}

func promptResult(title, instructions string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Description: title,
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.TextContent{Type: "text", Text: instructions},
			},
		},
	}
}
