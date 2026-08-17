package tools

import (
	"strings"
	"testing"
)

// TestBambooToolsRejectMissingArguments checks that every tool with a required
// parameter refuses to call Bamboo without it, and says which parameter is
// missing rather than failing somewhere downstream.
func TestBambooToolsRejectMissingArguments(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	cases := []struct {
		tool    string
		args    map[string]interface{}
		wantMsg string
	}{
		{"bamboo_get_project", map[string]interface{}{}, "projectKey is required"},
		{"bamboo_get_plan", map[string]interface{}{}, "planKey is required"},
		{"bamboo_search_plans", map[string]interface{}{}, "searchTerm is required"},
		{"bamboo_list_plan_branches", map[string]interface{}{}, "planKey is required"},
		{"bamboo_get_plan_branch", map[string]interface{}{"planKey": testPlanKey}, "branchName is required"},
		{"bamboo_get_latest_result", map[string]interface{}{}, "planKey is required"},
		{"bamboo_list_build_results", map[string]interface{}{}, "planKey is required"},
		{"bamboo_get_build_result", map[string]interface{}{}, "buildKey is required"},
		{"bamboo_get_build_result_expanded", map[string]interface{}{}, "buildKey is required"},
		{"bamboo_get_build_repositories", map[string]interface{}{}, "buildKey is required"},
		{"bamboo_get_plan_repositories", map[string]interface{}{}, "planKey is required"},
		{"bamboo_list_deployment_projects_for_plan", map[string]interface{}{}, "planKey is required"},
		{"bamboo_get_deployment_project", map[string]interface{}{}, "projectId is required"},
		{"bamboo_get_environment_results", map[string]interface{}{}, "environmentId is required"},
		{"bamboo_get_deployment_result", map[string]interface{}{}, "resultId is required"},
		{"bamboo_list_deploy_versions", map[string]interface{}{}, "projectId is required"},
		{"bamboo_get_deploy_version", map[string]interface{}{}, "versionId is required"},
		{"bamboo_get_deploy_version_status", map[string]interface{}{}, "versionId is required"},
	}

	handlers := Handlers(bambooTools, nil, nil)

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			handler, ok := handlers[tc.tool]
			if !ok {
				t.Fatalf("no handler registered for %q", tc.tool)
			}

			res, err := handler(tc.args)
			assertToolError(t, res, err, tc.wantMsg)

			if len(fake.paths()) != 0 {
				t.Errorf("tool called Bamboo despite a missing argument: %v", fake.paths())
			}
		})
	}
}

func TestGetServerInfoReturnsVersion(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	res, err := bambooTools.GetServerInfo(map[string]interface{}{})
	payload := assertOK(t, res, err)

	info := assertJSON(t, payload)
	if info["version"] != "9.2.7" {
		t.Errorf("version = %v, want 9.2.7", info["version"])
	}
	if !fake.requested("info") {
		t.Errorf("did not call the info endpoint; called %v", fake.paths())
	}
}

func TestHealthCheckReportsReachability(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	res, err := bambooTools.HealthCheck(map[string]interface{}{})
	payload := assertOK(t, res, err)

	if !strings.Contains(strings.ToLower(payload), "healthy") {
		t.Errorf("health check payload = %q, want it to report health", payload)
	}
}

func TestListAllProjectsCollectsEveryPage(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	res, err := bambooTools.ListAllProjects(map[string]interface{}{})
	payload := assertOK(t, res, err)

	if !strings.Contains(payload, testProjectKey) {
		t.Errorf("payload = %q, want it to contain project %q", payload, testProjectKey)
	}
	// The fixture reports a total size of 1, so pagination must stop after one
	// request rather than looping.
	if got := len(fake.paths()); got != 1 {
		t.Errorf("made %d requests (%v), want 1", got, fake.paths())
	}
}

func TestListDeploymentProjectsTruncatesClientSide(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	// The fixture returns two deployment projects; asking for one must truncate.
	res, err := bambooTools.ListDeploymentProjects(map[string]interface{}{"maxResult": float64(1)})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	if count, _ := result["returnedCount"].(float64); count != 1 {
		t.Errorf("returnedCount = %v, want 1", result["returnedCount"])
	}
}

func TestGetBuildResultExpandedAddsBambooLinks(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	res, err := bambooTools.GetBuildResultExpanded(map[string]interface{}{"buildKey": testBuildKey})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	links, ok := result["_links"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload has no _links object: %s", payload)
	}

	wantLog := fake.URL + "/browse/" + testBuildKey + "/log"
	if links["log_view"] != wantLog {
		t.Errorf("log_view = %v, want %v", links["log_view"], wantLog)
	}
}

func TestGetBuildRepositoriesEnrichesHelmRevisions(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	res, err := bambooTools.GetBuildRepositories(map[string]interface{}{"buildKey": testBuildKey})
	payload := assertOK(t, res, err)

	result := assertJSON(t, payload)
	repos, ok := result["repositories"].([]interface{})
	if !ok {
		t.Fatalf("payload has no repositories array: %s", payload)
	}
	// The fixture has one helm-shaped revision and one that must be ignored.
	if len(repos) != 1 {
		t.Fatalf("got %d repositories, want 1: %s", len(repos), payload)
	}

	repo, _ := repos[0].(map[string]interface{})
	if repo["name"] != "checkout" {
		t.Errorf("repo name = %v, want checkout", repo["name"])
	}
	if repo["commit"] != "abc123def456" {
		t.Errorf("repo commit = %v, want abc123def456", repo["commit"])
	}
	wantDiff := testBitbucket + "/projects/EXAMPLE/repos/checkout/commits/abc123def456"
	if repo["commit_url"] != wantDiff {
		t.Errorf("commit_url = %v, want %v", repo["commit_url"], wantDiff)
	}
}

func TestToolSurfacesUpstreamFailure(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, _, _ := newTestTools(t, fake)

	// The fake has no fixture for this plan, so Bamboo answers 404.
	res, err := bambooTools.GetPlan(map[string]interface{}{"planKey": "EXAMPLE-NOSUCHPLAN"})
	assertToolError(t, res, err, "Failed to get plan")

	if !strings.Contains(resultText(t, res), "404") {
		t.Errorf("error = %q, want it to include the upstream status", resultText(t, res))
	}
}

func TestParseHelmRepo(t *testing.T) {
	cases := []struct {
		name        string
		repoName    string
		revision    string
		bitbucket   string
		wantNil     bool
		wantRepo    string
		wantProject string
		wantBranch  string
		wantRepoURL string
	}{
		{
			name:        "helm repo with simple branch",
			repoName:    "PLATFORM.example.checkout.main.app",
			revision:    "abc123",
			bitbucket:   testBitbucket,
			wantRepo:    "checkout",
			wantProject: "EXAMPLE",
			wantBranch:  "main",
			wantRepoURL: testBitbucket + "/projects/EXAMPLE/repos/checkout/browse",
		},
		{
			name:        "branch containing dots is rejoined",
			repoName:    "PLATFORM.example.checkout.release.1.4.app",
			revision:    "abc123",
			bitbucket:   testBitbucket,
			wantRepo:    "checkout",
			wantProject: "EXAMPLE",
			wantBranch:  "release.1.4",
			wantRepoURL: testBitbucket + "/projects/EXAMPLE/repos/checkout/browse",
		},
		{
			name:      "no bitbucket url leaves links empty",
			repoName:  "PLATFORM.example.checkout.main.app",
			revision:  "abc123",
			bitbucket: "",
			wantRepo:  "checkout", wantProject: "EXAMPLE", wantBranch: "main",
			wantRepoURL: "",
		},
		{name: "too few segments", repoName: "PLATFORM.example.app", wantNil: true},
		{name: "wrong trailing segment", repoName: "PLATFORM.example.checkout.main.chart", wantNil: true},
		{name: "empty name", repoName: "", wantNil: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseHelmRepo(tc.repoName, tc.revision, tc.bitbucket)

			if tc.wantNil {
				if got != nil {
					t.Fatalf("parseHelmRepo() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("parseHelmRepo() = nil, want a repo")
			}
			if got.Name != tc.wantRepo {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantRepo)
			}
			if got.Project != tc.wantProject {
				t.Errorf("Project = %q, want %q", got.Project, tc.wantProject)
			}
			if got.Branch != tc.wantBranch {
				t.Errorf("Branch = %q, want %q", got.Branch, tc.wantBranch)
			}
			if got.RepoURL != tc.wantRepoURL {
				t.Errorf("RepoURL = %q, want %q", got.RepoURL, tc.wantRepoURL)
			}
		})
	}
}
