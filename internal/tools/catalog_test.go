package tools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hmdmph/bamboo-mcp/internal/storage"
)

// toolCases holds a representative, valid invocation for every tool in the
// catalog. TestEveryDeclaredToolIsExercised fails if a declared tool is missing
// from this table, so adding a tool without a test is not possible.
var toolCases = map[string]map[string]interface{}{
	// Server & projects
	"bamboo_server_info":       {},
	"bamboo_health_check":      {},
	"bamboo_list_projects":     {"maxResult": float64(25), "startIndex": float64(0)},
	"bamboo_list_all_projects": {},
	"bamboo_get_project":       {"projectKey": testProjectKey},

	// Plans & branches
	"bamboo_list_plans":         {},
	"bamboo_get_plan":           {"planKey": testPlanKey},
	"bamboo_search_plans":       {"searchTerm": "checkout"},
	"bamboo_list_plan_branches": {"planKey": testPlanKey},
	"bamboo_get_plan_branch":    {"planKey": testPlanKey, "branchName": testBranchName},

	// Builds
	"bamboo_get_latest_result":         {"planKey": testPlanKey},
	"bamboo_list_build_results":        {"planKey": testPlanKey, "maxResults": float64(5)},
	"bamboo_get_build_result":          {"buildKey": testBuildKey},
	"bamboo_get_build_result_expanded": {"buildKey": testBuildKey},
	"bamboo_get_build_repositories":    {"buildKey": testBuildKey},
	"bamboo_get_plan_repositories":     {"planKey": testPlanKey},
	"bamboo_get_build_queue":           {},

	// Deployments
	"bamboo_list_deployment_projects":          {"maxResult": float64(10)},
	"bamboo_list_deployment_projects_for_plan": {"planKey": testPlanKey},
	"bamboo_get_deployment_project":            {"projectId": testDeployProjID},
	"bamboo_get_environment_results":           {"environmentId": testEnvID, "maxResults": float64(10)},
	"bamboo_get_deployment_result":             {"resultId": testDeployResID},
	"bamboo_list_deploy_versions":              {"projectId": testDeployProjID, "maxResults": float64(10)},
	"bamboo_get_deploy_version":                {"versionId": testVersionID},
	"bamboo_get_deploy_version_status":         {"versionId": testVersionID},

	// Plan context
	"bamboo_get_plan_context":    {},
	"bamboo_reload_context":      {},
	"bamboo_resolve_plan":        {"reference": "CHECKOUT", "planType": "app"},
	"bamboo_get_deploy_status":   {"reference": "CHECKOUT", "planType": "app"},
	"bamboo_explain_environment": {"environmentName": "staging_network_checkout_deploy", "planType": "infra"},
	"bamboo_get_plan_type_info":  {"planType": "app"},

	// Bitbucket credential storage
	"bitbucket_add_repo": {
		"name":     "added-repo",
		"url":      testBitbucket + "/scm/example/checkout.git",
		"username": "dscully",
		"token":    "bitbucket-token-1234",
	},
	"bitbucket_get_repo":    {"name": "seeded-for-get"},
	"bitbucket_list_repos":  {},
	"bitbucket_delete_repo": {"name": "seeded-for-delete"},
}

// TestEveryDeclaredToolIsExercised calls every tool in the catalog through its
// registered handler and checks that it returns a usable, non-error payload.
// This is the test that keeps the declared tool surface honest: a tool that is
// advertised but broken, unrouted, or untested fails here.
func TestEveryDeclaredToolIsExercised(t *testing.T) {
	fake := newFakeBamboo(t)
	bambooTools, contextTools, bitbucketTools := newTestTools(t, fake)
	handlers := Handlers(bambooTools, contextTools, bitbucketTools)

	// The get/delete cases need something already stored, and each uses its own
	// entry so the subtests stay order-independent.
	seedRepos(t, bitbucketTools, "seeded-for-get", "seeded-for-delete")

	for _, def := range Catalog() {
		args, ok := toolCases[def.Name]
		if !ok {
			t.Errorf("tool %q is declared in the catalog but no case in toolCases exercises it", def.Name)
			continue
		}

		handler, ok := handlers[def.Name]
		if !ok {
			t.Errorf("tool %q is declared in the catalog but has no handler", def.Name)
			continue
		}

		t.Run(def.Name, func(t *testing.T) {
			res, err := handler(args)
			if payload := assertOK(t, res, err); strings.TrimSpace(payload) == "" {
				t.Error("tool returned an empty payload")
			}
		})
	}
}

// seedRepos stores throwaway Bitbucket credentials under the given names.
func seedRepos(t *testing.T, bitbucketTools *BitbucketTools, names ...string) {
	t.Helper()

	for _, name := range names {
		err := bitbucketTools.storage.AddRepo(name, &storage.BitbucketRepo{
			URL:      testBitbucket + "/scm/example/" + name + ".git",
			Username: "dscully",
			Token:    "bitbucket-token-5678",
		})
		if err != nil {
			t.Fatalf("seeding repo %q: %v", name, err)
		}
	}
}

// TestCatalogIsWellFormed checks the properties an MCP client relies on: unique
// names, a description on every tool, and required parameters that actually
// exist in the input schema.
func TestCatalogIsWellFormed(t *testing.T) {
	catalog := Catalog()

	if len(catalog) == 0 {
		t.Fatal("Catalog() returned no tools")
	}

	seen := make(map[string]bool, len(catalog))
	for _, def := range catalog {
		t.Run(def.Name, func(t *testing.T) {
			if def.Name == "" {
				t.Fatal("tool has an empty name")
			}
			if seen[def.Name] {
				t.Fatalf("tool name %q is declared twice", def.Name)
			}
			seen[def.Name] = true

			if strings.TrimSpace(def.Description) == "" {
				t.Error("tool has no description, so a model cannot tell when to call it")
			}
			if def.InputSchema.Type != "object" {
				t.Errorf("input schema type = %q, want \"object\"", def.InputSchema.Type)
			}

			for _, required := range def.InputSchema.Required {
				if _, ok := def.InputSchema.Properties[required]; !ok {
					t.Errorf("parameter %q is required but is not declared in properties", required)
				}
			}

			for name, raw := range def.InputSchema.Properties {
				prop, ok := raw.(map[string]interface{})
				if !ok {
					t.Errorf("parameter %q has a malformed schema (%T)", name, raw)
					continue
				}
				if desc, _ := prop["description"].(string); strings.TrimSpace(desc) == "" {
					t.Errorf("parameter %q has no description", name)
				}
			}
		})
	}
}

// TestCatalogAndHandlersAgree checks that declarations and implementations are
// one-to-one. Registration in cmd/server aborts on a mismatch, so catching it
// here keeps the failure at test time rather than at startup.
func TestCatalogAndHandlersAgree(t *testing.T) {
	fake := newFakeBamboo(t)
	handlers := Handlers(newTestTools(t, fake))

	catalog := Catalog()
	if len(handlers) != len(catalog) {
		t.Errorf("catalog declares %d tools but %d handlers are registered", len(catalog), len(handlers))
	}

	declared := make(map[string]bool, len(catalog))
	for _, def := range catalog {
		declared[def.Name] = true
		if handlers[def.Name] == nil {
			t.Errorf("tool %q is declared but has no handler", def.Name)
		}
	}

	for name := range handlers {
		if !declared[name] {
			t.Errorf("handler %q is registered but the tool is not declared in the catalog", name)
		}
	}
}

// TestToolsJSONIsCurrent guards the generated manifest in the repo root, which
// is what registries and scanners read to learn the tool surface without running
// the server. Regenerate it with `make tools.json`.
func TestToolsJSONIsCurrent(t *testing.T) {
	const manifest = "../../tools.json"

	onDisk, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("reading %s: %v (run `make tools.json`)", manifest, err)
	}

	generated, err := CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON() error = %v", err)
	}

	if strings.TrimSpace(string(onDisk)) != strings.TrimSpace(string(generated)) {
		t.Errorf("%s is out of date with the tool catalog; run `make tools.json`", manifest)
	}
}

// TestCatalogJSONMatchesMCPShape checks that the manifest is the payload an MCP
// client would receive from tools/list.
func TestCatalogJSONMatchesMCPShape(t *testing.T) {
	payload, err := CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON() error = %v", err)
	}

	var decoded struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string                 `json:"type"`
				Properties map[string]interface{} `json:"properties"`
				Required   []string               `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}

	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("catalog JSON does not decode as a tools/list payload: %v", err)
	}

	if len(decoded.Tools) != len(Catalog()) {
		t.Fatalf("catalog JSON has %d tools, want %d", len(decoded.Tools), len(Catalog()))
	}

	for _, tool := range decoded.Tools {
		if tool.Name == "" || tool.Description == "" || tool.InputSchema.Type != "object" {
			t.Errorf("tool %+v is missing a name, description, or object input schema", tool)
		}
	}
}
