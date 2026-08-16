//go:build integration
// +build integration

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/hmdmph/bamboo-mcp/internal/storage"
)

// Colors
const (
	red    = "\033[0;31m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
	cyan   = "\033[0;36m"
	nc     = "\033[0m"
)

var (
	pass int
	fail int
	skip int
)

func logInfo(msg string) { fmt.Fprintf(os.Stderr, "%s[INFO]%s  %s\n", cyan, nc, msg) }
func logPass(msg string) { fmt.Fprintf(os.Stderr, "%s[PASS]%s  %s\n", green, nc, msg); pass++ }
func logFail(msg string) { fmt.Fprintf(os.Stderr, "%s[FAIL]%s  %s\n", red, nc, msg); fail++ }
func logSkip(msg string) { fmt.Fprintf(os.Stderr, "%s[SKIP]%s  %s\n", yellow, nc, msg); skip++ }

func jsonPretty(data interface{}) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(b)
}

func getField(data map[string]interface{}, path string) string {
	keys := strings.Split(path, ".")
	var current interface{} = data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "N/A"
		}
		current, ok = m[key]
		if !ok {
			return "N/A"
		}
	}
	return fmt.Sprintf("%v", current)
}

func testMap(client *bamboo.Client, name string, fn func() (map[string]interface{}, error), fields ...string) {
	logInfo("Testing: " + name)
	start := time.Now()
	result, err := fn()
	elapsed := time.Since(start)
	if err != nil {
		logFail(fmt.Sprintf("%s - %v (%s)", name, err, elapsed))
		return
	}
	logPass(fmt.Sprintf("%s (%s)", name, elapsed))
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(os.Stderr, "    %s: %s\n", fields[i], getField(result, fields[i+1]))
	}
}

func testSlice(client *bamboo.Client, name string, fn func() ([]interface{}, error)) {
	logInfo("Testing: " + name)
	start := time.Now()
	result, err := fn()
	elapsed := time.Since(start)
	if err != nil {
		logFail(fmt.Sprintf("%s - %v (%s)", name, err, elapsed))
		return
	}
	logPass(fmt.Sprintf("%s (%s)", name, elapsed))
	fmt.Fprintf(os.Stderr, "    Count: %d\n", len(result))
}

func testBool(name string, fn func() (bool, error)) {
	logInfo("Testing: " + name)
	start := time.Now()
	result, err := fn()
	elapsed := time.Since(start)
	if !result || err != nil {
		logFail(fmt.Sprintf("%s - healthy=%v err=%v (%s)", name, result, err, elapsed))
		return
	}
	logPass(fmt.Sprintf("%s (%s)", name, elapsed))
}

func testAllProjects(client *bamboo.Client) {
	logInfo("Testing: List All Projects")
	start := time.Now()
	result, err := client.ListAllProjects()
	elapsed := time.Since(start)
	if err != nil {
		logFail(fmt.Sprintf("List All Projects - %v (%s)", err, elapsed))
		return
	}
	logPass(fmt.Sprintf("List All Projects (%s)", elapsed))
	fmt.Fprintf(os.Stderr, "    Total projects: %d\n", len(result))
}

func testBitbucketStorage() {
	logInfo("Testing: Bitbucket Storage CRUD")

	store, err := storage.NewBitbucketStorage()
	if err != nil {
		logFail(fmt.Sprintf("Bitbucket Storage init - %v", err))
		return
	}

	repo := &storage.BitbucketRepo{
		URL:      "https://bitbucket.org/test/integration-test-repo",
		Username: "testuser",
		Token:    "testtoken123",
	}

	// Add
	if err := store.AddRepo("integration-test-repo", repo); err != nil {
		logFail(fmt.Sprintf("Bitbucket Add Repo - %v", err))
		return
	}
	logPass("Bitbucket Add Repo")

	// Get
	got, err := store.GetRepo("integration-test-repo")
	if err != nil {
		logFail(fmt.Sprintf("Bitbucket Get Repo - %v", err))
		return
	}
	if got.URL != repo.URL || got.Username != repo.Username {
		logFail(fmt.Sprintf("Bitbucket Get Repo - data mismatch: got URL=%s", got.URL))
		return
	}
	logPass("Bitbucket Get Repo")

	// List
	repos := store.ListRepos()
	if _, ok := repos["integration-test-repo"]; !ok {
		logFail("Bitbucket List Repos - test repo not found in list")
		return
	}
	logPass("Bitbucket List Repos")

	// Delete
	if err := store.DeleteRepo("integration-test-repo"); err != nil {
		logFail(fmt.Sprintf("Bitbucket Delete Repo - %v", err))
		return
	}
	_, err = store.GetRepo("integration-test-repo")
	if err == nil {
		logFail("Bitbucket Delete Repo - repo still exists after delete")
		return
	}
	logPass("Bitbucket Delete Repo")
}

func main() {
	bambooURL := os.Getenv("BAMBOO_URL")
	bambooToken := os.Getenv("BAMBOO_TOKEN")
	bambooProxy := os.Getenv("BAMBOO_PROXY")
	planKey := os.Getenv("PLAN_KEY")
	projectKey := os.Getenv("PROJECT_KEY")
	verbose := strings.ToLower(os.Getenv("VERBOSE"))

	if bambooURL == "" || bambooToken == "" {
		fmt.Fprintf(os.Stderr, "%sERROR: BAMBOO_URL and BAMBOO_TOKEN are required%s\n", red, nc)
		fmt.Fprintf(os.Stderr, "Usage: BAMBOO_URL=... BAMBOO_TOKEN=... go run scripts/integration_test.go\n")
		os.Exit(1)
	}

	if verbose == "true" || verbose == "1" {
		logger.SetVerbose(true)
	}

	client, err := bamboo.NewClient(bambooURL, bambooToken, bambooProxy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sERROR: Failed to create client: %v%s\n", red, err, nc)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "=========================================")
	fmt.Fprintln(os.Stderr, "  Bamboo MCP Server - Integration Tests  ")
	fmt.Fprintln(os.Stderr, "=========================================")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  BAMBOO_URL:   %s\n", bambooURL)
	fmt.Fprintf(os.Stderr, "  PROJECT_KEY:  %s\n", orDefault(projectKey, "(not set)"))
	fmt.Fprintf(os.Stderr, "  PLAN_KEY:     %s\n", orDefault(planKey, "(not set)"))
	fmt.Fprintln(os.Stderr)

	// --- Bamboo API Tests ---
	fmt.Fprintln(os.Stderr, "-----------------------------------------")
	fmt.Fprintln(os.Stderr, "  Bamboo API Tests")
	fmt.Fprintln(os.Stderr, "-----------------------------------------")

	testBool("Health Check", client.HealthCheck)

	testMap(client, "Server Info", client.GetServerInfo,
		"Version", "version", "State", "state")

	testMap(client, "List Projects", func() (map[string]interface{}, error) {
		return client.ListProjects(5, 0)
	}, "Total", "projects.size")

	testAllProjects(client)

	if projectKey != "" {
		testMap(client, "Get Project ("+projectKey+")", func() (map[string]interface{}, error) {
			return client.GetProject(projectKey)
		}, "Name", "name", "Key", "key")
	} else {
		logSkip("Get Project - PROJECT_KEY not set")
	}

	testMap(client, "List Plans", client.ListPlans,
		"Total", "plans.size")

	testMap(client, "Search Plans", func() (map[string]interface{}, error) {
		return client.SearchPlans("build")
	}, "Results", "size")

	if planKey != "" {
		testMap(client, "Get Plan ("+planKey+")", func() (map[string]interface{}, error) {
			return client.GetPlan(planKey)
		}, "Name", "name", "Enabled", "enabled")

		testMap(client, "List Plan Branches ("+planKey+")", func() (map[string]interface{}, error) {
			return client.ListPlanBranches(planKey)
		}, "Branches", "branches.size")

		testMap(client, "Get Latest Result ("+planKey+")", func() (map[string]interface{}, error) {
			return client.GetLatestResult(planKey)
		}, "Build", "buildResultKey", "State", "state")

		testMap(client, "List Build Results ("+planKey+")", func() (map[string]interface{}, error) {
			return client.ListBuildResults(planKey, 5)
		}, "Results", "results.size")
	} else {
		logSkip("Get Plan - PLAN_KEY not set")
		logSkip("List Plan Branches - PLAN_KEY not set")
		logSkip("Get Latest Result - PLAN_KEY not set")
		logSkip("List Build Results - PLAN_KEY not set")
	}

	testMap(client, "Get Build Queue", client.GetBuildQueue,
		"Queue Size", "queuedBuilds.size")

	testSlice(client, "List Deployment Projects (max 10)", func() ([]interface{}, error) {
		return client.ListDeploymentProjects(10)
	})

	if planKey != "" {
		testSlice(client, "List Deployment Projects for Plan ("+planKey+")", func() ([]interface{}, error) {
			return client.ListDeploymentProjectsForPlan(planKey)
		})
	} else {
		logSkip("List Deployment Projects for Plan - PLAN_KEY not set")
	}

	// --- Bitbucket Storage Tests ---
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "-----------------------------------------")
	fmt.Fprintln(os.Stderr, "  Bitbucket Storage Tests")
	fmt.Fprintln(os.Stderr, "-----------------------------------------")

	testBitbucketStorage()

	// --- Results ---
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "=========================================")
	fmt.Fprintln(os.Stderr, "  Results")
	fmt.Fprintln(os.Stderr, "=========================================")
	fmt.Fprintf(os.Stderr, "  %sPASS: %d%s\n", green, pass, nc)
	fmt.Fprintf(os.Stderr, "  %sFAIL: %d%s\n", red, fail, nc)
	fmt.Fprintf(os.Stderr, "  %sSKIP: %d%s\n", yellow, skip, nc)
	fmt.Fprintln(os.Stderr, "=========================================")
	fmt.Fprintln(os.Stderr)

	if fail > 0 {
		fmt.Fprintf(os.Stderr, "%sSome tests failed!%s\n", red, nc)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%sAll tests passed!%s\n", green, nc)
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
