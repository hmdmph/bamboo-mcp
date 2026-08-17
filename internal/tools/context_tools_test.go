package tools

import (
	"strings"
	"testing"
)

func TestContextToolsRejectMissingArguments(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	cases := []struct {
		tool    string
		args    map[string]interface{}
		wantMsg string
	}{
		{"bamboo_resolve_plan", map[string]interface{}{}, "reference is required"},
		{"bamboo_get_deploy_status", map[string]interface{}{}, "reference is required"},
		{"bamboo_explain_environment", map[string]interface{}{}, "environmentName is required"},
		{"bamboo_get_plan_type_info", map[string]interface{}{}, "planType is required"},
	}

	handlers := Handlers(nil, contextTools, nil)

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			res, err := handlers[tc.tool](tc.args)
			assertToolError(t, res, err, tc.wantMsg)
		})
	}
}

func TestResolvePlanAppliesNamingConvention(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	// The default context resolves EXAMPLE + CHECKOUT + the "app" suffix.
	res, err := contextTools.ResolvePlan(map[string]interface{}{"reference": "checkout", "planType": "app"})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	if result["resolved_plan_key"] != testPlanKey {
		t.Errorf("resolved_plan_key = %v, want %v", result["resolved_plan_key"], testPlanKey)
	}
	if result["resolved_type"] != "app" {
		t.Errorf("resolved_type = %v, want app", result["resolved_type"])
	}
	if _, ok := result["plan"]; !ok {
		t.Errorf("payload has no plan details: %s", payload)
	}
}

func TestResolvePlanFallsBackToSearch(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	// UNKNOWN resolves to a key the fake does not serve, so the tool must fall
	// back to a plan search rather than reporting failure.
	res, err := contextTools.ResolvePlan(map[string]interface{}{"reference": "unknown", "planType": "app"})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	if result["direct_lookup"] != "failed" {
		t.Errorf("direct_lookup = %v, want failed", result["direct_lookup"])
	}
	if _, ok := result["search_results"]; !ok {
		t.Errorf("payload has no search_results: %s", payload)
	}
	if !fake.requested("search/plans") {
		t.Errorf("did not fall back to a plan search; called %v", fake.paths())
	}
}

func TestGetDeployStatusAssemblesBuildAndDeployments(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.GetDeployStatus(map[string]interface{}{"reference": "checkout", "planType": "app"})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	if result["plan_key"] != testPlanKey {
		t.Errorf("plan_key = %v, want %v", result["plan_key"], testPlanKey)
	}

	build, ok := result["latest_build"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload has no latest_build: %s", payload)
	}
	if build["buildState"] != "Successful" {
		t.Errorf("buildState = %v, want Successful", build["buildState"])
	}

	deployments, ok := result["deployments"].([]interface{})
	if !ok || len(deployments) != 1 {
		t.Fatalf("want one deployment project, got: %s", payload)
	}

	project, _ := deployments[0].(map[string]interface{})
	envs, _ := project["matched_environments"].([]interface{})
	if len(envs) != 2 {
		t.Fatalf("want both environments unfiltered, got %d: %s", len(envs), payload)
	}

	// The prod environment has deployment history; the staging one does not, and
	// must be reported without a latest_deployment rather than dropped.
	var withHistory, withoutHistory int
	for _, e := range envs {
		env, _ := e.(map[string]interface{})
		if _, ok := env["latest_deployment"]; ok {
			withHistory++
		} else {
			withoutHistory++
		}
	}
	if withHistory != 1 || withoutHistory != 1 {
		t.Errorf("environments with/without history = %d/%d, want 1/1", withHistory, withoutHistory)
	}

	links, _ := result["links"].(map[string]interface{})
	wantBuild := fake.URL + "/browse/" + testBuildKey
	if links["latest_build"] != wantBuild {
		t.Errorf("latest_build link = %v, want %v", links["latest_build"], wantBuild)
	}
}

func TestGetDeployStatusFiltersEnvironments(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.GetDeployStatus(map[string]interface{}{
		"reference":   "checkout",
		"planType":    "app",
		"environment": "prod",
	})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	deployments, _ := result["deployments"].([]interface{})
	project, _ := deployments[0].(map[string]interface{})

	envs, _ := project["matched_environments"].([]interface{})
	if len(envs) != 1 {
		t.Fatalf("want only the prod environment, got %d: %s", len(envs), payload)
	}
	env, _ := envs[0].(map[string]interface{})
	if env["name"] != "prod_deploy" {
		t.Errorf("environment = %v, want prod_deploy", env["name"])
	}

	// The unfiltered total must still be reported so the caller knows what was
	// filtered out.
	if total, _ := project["total_environments"].(float64); total != 2 {
		t.Errorf("total_environments = %v, want 2", project["total_environments"])
	}

	deployment, ok := env["latest_deployment"].(map[string]interface{})
	if !ok {
		t.Fatalf("prod environment has no latest_deployment: %s", payload)
	}
	if deployment["status"] != "SUCCESS" {
		t.Errorf("status = %v, want SUCCESS", deployment["status"])
	}
	if deployment["triggered_by"] != "Manual run by Dana Scully" {
		t.Errorf("triggered_by = %v, want the reason summary", deployment["triggered_by"])
	}
	version, _ := deployment["version"].(map[string]interface{})
	if version["name"] != "release-1.4.0" {
		t.Errorf("version name = %v, want release-1.4.0", version["name"])
	}
}

func TestExplainEnvironmentDecomposesName(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.ExplainEnvironment(map[string]interface{}{
		"environmentName": "staging_network_checkout_deploy",
		"planType":        "infra",
	})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	parsed, ok := result["parsed"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload has no parsed object: %s", payload)
	}

	want := map[string]string{
		"raw":                "staging_network_checkout_deploy",
		"environment":        "staging",
		"module":             "network",
		"ref":                "checkout",
		"action":             "deploy",
		"module_description": "Networking resources (VPCs, subnets, load balancers)",
	}
	for field, value := range want {
		if parsed[field] != value {
			t.Errorf("parsed[%q] = %v, want %q", field, parsed[field], value)
		}
	}

	if parsed["plan_type"] != "infra" {
		t.Errorf("plan_type = %v, want infra", parsed["plan_type"])
	}
}

func TestExplainEnvironmentHintsWhenPlanTypeOmitted(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.ExplainEnvironment(map[string]interface{}{"environmentName": "prod_deploy"})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	hint, _ := result["hint"].(string)
	if !strings.Contains(hint, "planType") {
		t.Errorf("hint = %q, want it to suggest passing planType", hint)
	}
}

func TestGetPlanTypeInfoDescribesConventions(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.GetPlanTypeInfo(map[string]interface{}{"planType": "infra"})
	payload := assertOK(t, res, err)

	info := assertJSON(t, payload)
	if info["type"] != "infra" {
		t.Errorf("type = %v, want infra", info["type"])
	}
	if info["environment_format"] != "<env>_<module>_<ref>_<action>" {
		t.Errorf("environment_format = %v, want the infra format", info["environment_format"])
	}
	if modules, _ := info["modules"].([]interface{}); len(modules) == 0 {
		t.Errorf("infra plan type reports no modules: %s", payload)
	}
	if actions, _ := info["actions"].([]interface{}); len(actions) == 0 {
		t.Errorf("infra plan type reports no actions: %s", payload)
	}
}

func TestGetPlanTypeInfoListsAvailableTypesOnMiss(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.GetPlanTypeInfo(map[string]interface{}{"planType": "nonesuch"})
	assertToolError(t, res, err, "Unknown plan type")

	// The error is what the model sees, so it has to name the valid options.
	if text := resultText(t, res); !strings.Contains(text, "app") || !strings.Contains(text, "infra") {
		t.Errorf("error = %q, want it to list the available plan types", text)
	}
}

func TestGetPlanContextAndReload(t *testing.T) {
	fake := newFakeBamboo(t)
	_, contextTools, _ := newTestTools(t, fake)

	res, err := contextTools.GetPlanContext(map[string]interface{}{})
	summary := assertJSON(t, assertOK(t, res, err))

	if summary["default_project"] != testProjectKey {
		t.Errorf("default_project = %v, want %v", summary["default_project"], testProjectKey)
	}

	// Reload re-reads the file NewPlanContext wrote, so the summary must survive
	// a round trip through YAML unchanged.
	reloadRes, reloadErr := contextTools.ReloadContext(map[string]interface{}{})
	reloaded := assertOK(t, reloadRes, reloadErr)

	if !strings.Contains(reloaded, "reloaded successfully") {
		t.Errorf("reload payload = %q, want a confirmation", reloaded)
	}
	if !strings.Contains(reloaded, testProjectKey) {
		t.Errorf("reload payload lost the default project: %s", reloaded)
	}
}
