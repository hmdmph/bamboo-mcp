package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	pctx "github.com/hmdmph/bamboo-mcp/internal/context"
	"github.com/hmdmph/bamboo-mcp/internal/storage"
	"github.com/mark3labs/mcp-go/mcp"
)

// Fixture identifiers shared by every tool test. They line up with the default
// plan context: reference "CHECKOUT" + plan type "app" resolves to
// EXAMPLE-CHECKOUTAPP.
const (
	testProjectKey   = "EXAMPLE"
	testPlanKey      = "EXAMPLE-CHECKOUTAPP"
	testBranchName   = "main"
	testBuildKey     = "EXAMPLE-CHECKOUTAPP-42"
	testDeployProjID = "1"
	testEnvID        = "10"
	testDeployResID  = "100"
	testVersionID    = "200"
	testBitbucket    = "https://bitbucket.example.com"
)

// fakeBamboo is a stand-in for a self-hosted Bamboo REST API. It answers the
// endpoints the client actually calls and records the paths it was asked for, so
// a test can assert that a tool hit the endpoint it claims to.
type fakeBamboo struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string
}

// newFakeBamboo starts a fake Bamboo and shuts it down when the test ends.
func newFakeBamboo(t *testing.T) *fakeBamboo {
	t.Helper()

	f := &fakeBamboo{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/latest/")

		f.mu.Lock()
		f.requests = append(f.requests, path)
		f.mu.Unlock()

		// The client must always present the token as a bearer credential.
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		body, ok := fakeResponse(path)
		if !ok {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)

	return f
}

// paths returns the REST paths requested so far, in order.
func (f *fakeBamboo) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// requested reports whether the given REST path was called.
func (f *fakeBamboo) requested(path string) bool {
	for _, p := range f.paths() {
		if p == path {
			return true
		}
	}
	return false
}

// fakeResponse maps a Bamboo REST path to a canned JSON body. Ordering matters:
// the more specific paths are matched before their prefixes.
func fakeResponse(path string) (string, bool) {
	switch path {
	case "info":
		return `{"version":"9.2.7","edition":"","buildDate":"2025-01-15T00:00:00.000Z","buildNumber":"90207","state":"RUNNING"}`, true

	case "project":
		// size == 1 so ListAllProjects stops after a single page.
		return `{"projects":{"size":1,"max-result":25,"start-index":0,"project":[
			{"key":"EXAMPLE","name":"Example Project","link":{"href":"http://bamboo/rest/api/latest/project/EXAMPLE"}}
		]}}`, true

	case "plan":
		return `{"plans":{"size":1,"plan":[{"key":"EXAMPLE-CHECKOUTAPP","name":"Example - APPCHECKOUT (main) - App","shortName":"CHECKOUTAPP","enabled":true}]}}`, true

	case "search/plans":
		return `{"size":1,"searchResults":[{"id":"EXAMPLE-CHECKOUTAPP","type":"plan","searchEntity":{"key":"EXAMPLE-CHECKOUTAPP","projectName":"Example Project","planName":"Checkout App"}}]}`, true

	case "queue":
		return `{"expand":"queuedBuilds","queue":{"queuedBuilds":{"size":1,"queuedBuild":[{"planKey":"EXAMPLE-CHECKOUTAPP","buildNumber":43,"triggerReason":"Manual build"}]}}}`, true

	case "deploy/project/all":
		return `[{"id":1,"name":"Checkout App Deployment","planKey":{"key":"EXAMPLE-CHECKOUTAPP"}},
		         {"id":2,"name":"Payments App Deployment","planKey":{"key":"EXAMPLE-PAYMENTSAPP"}}]`, true

	case "deploy/project/forPlan":
		return `[{"id":1,"name":"Checkout App Deployment","planKey":{"key":"EXAMPLE-CHECKOUTAPP"}}]`, true

	case "deploy/project/" + testDeployProjID:
		return `{"id":1,"name":"Checkout App Deployment","planKey":{"key":"EXAMPLE-CHECKOUTAPP"},"environments":[
			{"id":10,"name":"prod_deploy","position":0},
			{"id":11,"name":"staging_deploy","position":1}
		]}`, true

	case "deploy/project/" + testDeployProjID + "/versions":
		return `{"versions":[{"id":200,"name":"release-1.4.0","creatorDisplayName":"Dana Scully","creationDate":1737000000000,"planBranchName":"main"}]}`, true

	case "deploy/environment/" + testEnvID + "/results":
		return `{"results":[{
			"id":100,
			"deploymentState":"SUCCESS",
			"lifeCycleState":"FINISHED",
			"reasonSummary":"Manual run by Dana Scully",
			"queuedDate":1737000000000,
			"startedDate":1737000010000,
			"executedDate":1737000010000,
			"finishedDate":1737000090000,
			"deploymentVersion":{"id":200,"name":"release-1.4.0","creatorDisplayName":"Dana Scully"}
		}]}`, true

	case "deploy/environment/11/results":
		// An environment that has never been deployed to.
		return `{"results":[]}`, true

	case "deploy/result/" + testDeployResID:
		return `{"id":100,"deploymentState":"SUCCESS","lifeCycleState":"FINISHED","reasonSummary":"Manual run by Dana Scully","agent":{"name":"agent-01"}}`, true

	case "deploy/version/" + testVersionID:
		return `{"id":200,"name":"release-1.4.0","creatorDisplayName":"Dana Scully","planBranchName":"main","items":[]}`, true

	case "deploy/version/" + testVersionID + "/status":
		return `[{"environmentId":10,"environmentName":"prod_deploy","deploymentState":"SUCCESS"}]`, true

	case "plan/" + testPlanKey:
		return `{"key":"EXAMPLE-CHECKOUTAPP","name":"Example - APPCHECKOUT (main) - App","enabled":true,
		         "repositories":{"size":1,"repository":[{"id":501,"name":"PLATFORM.example.checkout.main.app"}]}}`, true

	case "plan/" + testPlanKey + "/branch":
		return `{"branches":{"size":1,"branch":[{"key":"EXAMPLE-CHECKOUTAPP0","shortName":"main","enabled":true}]}}`, true

	case "plan/" + testPlanKey + "/branch/" + testBranchName:
		return `{"key":"EXAMPLE-CHECKOUTAPP0","shortName":"main","enabled":true}`, true

	case "project/" + testProjectKey:
		return `{"key":"EXAMPLE","name":"Example Project","description":"Fixture project"}`, true

	case "result/" + testPlanKey + "/latest":
		return `{"buildNumber":42,"buildState":"Successful","successful":true,"lifeCycleState":"Finished",
		         "buildResultKey":"EXAMPLE-CHECKOUTAPP-42","buildReason":"Manual build","buildDurationDescription":"1 minute"}`, true

	case "result/" + testPlanKey:
		// ListBuildResults asks for the plan-level result collection.
		return `{"results":{"size":1,"result":[{"buildNumber":42,"buildState":"Successful","key":"EXAMPLE-CHECKOUTAPP-42"}]}}`, true

	case "result/" + testBuildKey:
		// A single build. The expanded variant asks for the same path with extra
		// expand params, so this body carries the expanded fields too.
		return `{"buildNumber":42,"buildState":"Successful","successful":true,"buildResultKey":"EXAMPLE-CHECKOUTAPP-42",
			"reasonSummary":"Manual run by Dana Scully",
			"stages":{"stage":[{"name":"Build","state":"Successful"}]},
			"artifacts":{"artifact":[{"name":"chart.tgz"}]},
			"changes":{"change":[{"author":"dscully","comment":"Bump chart version"}]},
			"vcsRevisions":{"vcsRevision":[
				{"repositoryName":"PLATFORM.example.checkout.main.app","vcsRevisionKey":"abc123def456"},
				{"repositoryName":"not-a-helm-repo","vcsRevisionKey":"999999"}
			]}}`, true
	}

	return "", false
}

// newTestTools builds the three tool groups against the fake Bamboo, a temp
// plan-context file (which falls back to the built-in defaults) and temp-backed
// credential storage. Nothing touches the developer's real ~/.bamboo-mcp.
func newTestTools(t *testing.T, f *fakeBamboo) (*BambooTools, *ContextTools, *BitbucketTools) {
	t.Helper()

	client, err := bamboo.NewClient(f.URL, "test-token", "")
	if err != nil {
		t.Fatalf("bamboo.NewClient() error = %v", err)
	}

	planContext, err := pctx.NewPlanContext(filepath.Join(t.TempDir(), "context.yaml"))
	if err != nil {
		t.Fatalf("pctx.NewPlanContext() error = %v", err)
	}

	// NewBitbucketStorage resolves its path from the home directory, so point
	// HOME at a temp dir for the duration of the test.
	t.Setenv("HOME", t.TempDir())
	repoStorage, err := storage.NewBitbucketStorage()
	if err != nil {
		t.Fatalf("storage.NewBitbucketStorage() error = %v", err)
	}

	return NewBambooTools(client, testBitbucket),
		NewContextTools(client, planContext),
		NewBitbucketTools(repoStorage)
}

// resultText returns the text payload of a tool result, failing the test if the
// result is malformed.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	if res == nil {
		t.Fatal("tool returned a nil result")
	}
	if len(res.Content) == 0 {
		t.Fatal("tool returned no content")
	}

	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("tool returned %T content, want mcp.TextContent", res.Content[0])
	}

	return text.Text
}

// assertOK checks that a tool call succeeded and returns its text payload.
func assertOK(t *testing.T, res *mcp.CallToolResult, err error) string {
	t.Helper()

	if err != nil {
		t.Fatalf("tool returned a transport error: %v", err)
	}
	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("tool reported an error: %s", text)
	}

	return text
}

// assertToolError checks that a tool reported a user-facing error mentioning
// want, rather than succeeding or failing at the transport level.
func assertToolError(t *testing.T, res *mcp.CallToolResult, err error, want string) {
	t.Helper()

	if err != nil {
		t.Fatalf("tool returned a transport error: %v", err)
	}
	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("tool succeeded, want an error result: %s", text)
	}
	if !strings.Contains(text, want) {
		t.Errorf("error = %q, want it to mention %q", text, want)
	}
}

// assertJSON checks that a tool's payload is valid JSON and returns it decoded.
func assertJSON(t *testing.T, payload string) map[string]interface{} {
	t.Helper()

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("tool payload is not a JSON object: %v\npayload: %s", err, payload)
	}

	return decoded
}
