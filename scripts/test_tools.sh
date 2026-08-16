#!/usr/bin/env bash
#
# Integration test script for Bamboo MCP Server
# Tests all Bamboo REST API endpoints used by the MCP tools.
#
# Usage:
#   BAMBOO_URL=https://bamboo.example.com BAMBOO_TOKEN=your_token ./scripts/test_tools.sh
#
# Options:
#   BAMBOO_PROXY    Optional HTTP proxy URL
#   PLAN_KEY        Plan key to test with (default: auto-detected)
#   PROJECT_KEY     Project key to test with (default: auto-detected)
#   SEARCH_TERM     Search term for plan search (default: build)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$ROOT_DIR/bin/bamboo-mcp"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

PASS=0
FAIL=0
SKIP=0

# ---------- helpers ----------

log_info()  { echo -e "${CYAN}[INFO]${NC}  $*" >&2; }
log_pass()  { echo -e "${GREEN}[PASS]${NC}  $*" >&2; PASS=$((PASS + 1)); }
log_fail()  { echo -e "${RED}[FAIL]${NC}  $*" >&2; FAIL=$((FAIL + 1)); }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC}  $*" >&2; }

check_prereqs() {
    if [[ -z "${BAMBOO_URL:-}" ]]; then
        echo -e "${RED}ERROR: BAMBOO_URL environment variable is required${NC}"
        echo "Usage: BAMBOO_URL=https://bamboo.example.com BAMBOO_TOKEN=token $0"
        exit 1
    fi
    if [[ -z "${BAMBOO_TOKEN:-}" ]]; then
        echo -e "${RED}ERROR: BAMBOO_TOKEN environment variable is required${NC}"
        exit 1
    fi

    if ! command -v curl &> /dev/null; then
        echo -e "${RED}ERROR: curl is required${NC}"
        exit 1
    fi
    if ! command -v python3 &> /dev/null; then
        echo -e "${RED}ERROR: python3 is required${NC}"
        exit 1
    fi

    # Also verify the binary builds
    log_info "Building binary..."
    make -C "$ROOT_DIR" build 2>&1 | tail -1
}

# Base URL for REST API (strip trailing slash)
base_url() {
    echo "${BAMBOO_URL%/}/rest/api/latest"
}

# Make an authenticated GET request to the Bamboo REST API
# Returns: HTTP status code. Body is written to stdout.
bamboo_get() {
    local path="$1"
    local url="$(base_url)/${path}"
    local curl_opts=(-s -k -L -w "\n%{http_code}" -H "Authorization: Bearer ${BAMBOO_TOKEN}" -H "Accept: application/json" -H "X-Atlassian-Token: nocheck")

    if [[ -n "${BAMBOO_PROXY:-}" ]]; then
        curl_opts+=(-x "$BAMBOO_PROXY")
    fi

    curl "${curl_opts[@]}" "$url"
}

# Run a REST API test.
# Args: test_name, api_path
# Prints the JSON body on success, returns 0/1.
run_api_test() {
    local test_name="$1"
    local api_path="$2"

    log_info "Testing: $test_name"

    local raw_response
    raw_response=$(bamboo_get "$api_path")

    # Last line is HTTP status code
    local http_code
    http_code=$(echo "$raw_response" | tail -1)
    local body
    body=$(echo "$raw_response" | sed '$d')

    if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
        log_pass "$test_name (HTTP $http_code)"
        echo "$body"
        return 0
    else
        log_fail "$test_name (HTTP $http_code)"
        echo "$body" | head -5
        return 1
    fi
}

# Pretty-print a JSON field
print_field() {
    local json="$1"
    local label="$2"
    local jpath="$3"
    local val
    val=$(echo "$json" | python3 -c "
import sys, json
try:
    data = json.loads(sys.stdin.read())
    keys = '${jpath}'.split('.')
    v = data
    for k in keys:
        if isinstance(v, dict):
            v = v.get(k, 'N/A')
        else:
            v = 'N/A'
            break
    print(v)
except:
    print('N/A')
" 2>/dev/null)
    echo "    $label: $val" >&2
}

# ---------- test cases ----------

test_health_check() {
    local body
    if body=$(run_api_test "Health Check (server info)" "info"); then
        print_field "$body" "State" "state"
    fi
}

test_server_info() {
    local body
    if body=$(run_api_test "Server Info" "info"); then
        print_field "$body" "Version" "version"
        print_field "$body" "State" "state"
        print_field "$body" "Build Date" "buildDate"
    fi
}

test_list_projects() {
    local body
    if body=$(run_api_test "List Projects" "project?expand=projects.project&max-result=5"); then
        print_field "$body" "Total projects" "projects.size"
    fi
}

test_list_all_projects() {
    local body
    if body=$(run_api_test "List All Projects (page 1)" "project?expand=projects.project&max-result=25"); then
        local size
        size=$(echo "$body" | python3 -c "
import sys, json
data = json.loads(sys.stdin.read())
print(data.get('projects', {}).get('size', 0))
" 2>/dev/null)
        echo "    Projects on page 1: $size"
    fi
}

test_get_project() {
    local key="${PROJECT_KEY:-}"
    if [[ -z "$key" ]]; then
        log_skip "Get Project - PROJECT_KEY not set"
        return 0
    fi
    local body
    if body=$(run_api_test "Get Project ($key)" "project/$key"); then
        print_field "$body" "Name" "name"
        print_field "$body" "Key" "key"
    fi
}

test_list_plans() {
    local body
    if body=$(run_api_test "List Plans" "plan"); then
        print_field "$body" "Total plans" "plans.size"
    fi
}

test_search_plans() {
    local term="${SEARCH_TERM:-build}"
    local body
    if body=$(run_api_test "Search Plans ($term)" "search/plans?searchTerm=${term}"); then
        print_field "$body" "Results found" "size"
    fi
}

test_get_plan() {
    local key="${PLAN_KEY:-}"
    if [[ -z "$key" ]]; then
        log_skip "Get Plan - PLAN_KEY not set"
        return 0
    fi
    local body
    if body=$(run_api_test "Get Plan ($key)" "plan/$key"); then
        print_field "$body" "Name" "name"
        print_field "$body" "Enabled" "enabled"
    fi
}

test_list_plan_branches() {
    local key="${PLAN_KEY:-}"
    if [[ -z "$key" ]]; then
        log_skip "List Plan Branches - PLAN_KEY not set"
        return 0
    fi
    local body
    if body=$(run_api_test "List Plan Branches ($key)" "plan/$key/branch"); then
        print_field "$body" "Branch count" "branches.size"
    fi
}

test_get_latest_result() {
    local key="${PLAN_KEY:-}"
    if [[ -z "$key" ]]; then
        log_skip "Get Latest Result - PLAN_KEY not set"
        return 0
    fi
    local body
    if body=$(run_api_test "Get Latest Result ($key)" "result/$key/latest"); then
        print_field "$body" "Build" "buildResultKey"
        print_field "$body" "State" "state"
    fi
}

test_list_build_results() {
    local key="${PLAN_KEY:-}"
    if [[ -z "$key" ]]; then
        log_skip "List Build Results - PLAN_KEY not set"
        return 0
    fi
    local body
    if body=$(run_api_test "List Build Results ($key)" "result/$key?max-results=5"); then
        print_field "$body" "Results count" "results.size"
    fi
}

test_get_build_queue() {
    local body
    if body=$(run_api_test "Get Build Queue" "queue"); then
        print_field "$body" "Queue size" "queuedBuilds.size"
    fi
}

test_list_deployment_projects() {
    local body
    if body=$(run_api_test "List Deployment Projects" "deploy/project/all"); then
        local count
        count=$(echo "$body" | python3 -c "
import sys, json
try:
    data = json.loads(sys.stdin.read())
    if isinstance(data, list):
        print(len(data))
    else:
        print(1)
except:
    print('error')
" 2>/dev/null)
        echo "    Deployment projects: $count"
    fi
}

test_get_deployment_project() {
    local id="${DEPLOYMENT_PROJECT_ID:-}"
    if [[ -z "$id" ]]; then
        log_skip "Get Deployment Project - DEPLOYMENT_PROJECT_ID not set"
        return 0
    fi
    local body
    if body=$(run_api_test "Get Deployment Project ($id)" "deploy/project/$id"); then
        print_field "$body" "Name" "name"
    fi
}

test_binary_builds() {
    log_info "Testing: Binary builds successfully"
    if [[ -f "$BINARY" ]]; then
        log_pass "Binary exists at $BINARY"
    else
        log_fail "Binary not found at $BINARY"
    fi
}

test_unit_tests() {
    log_info "Testing: Unit tests pass"
    if (cd "$ROOT_DIR" && go test ./... > /dev/null 2>&1); then
        log_pass "All unit tests pass"
    else
        log_fail "Unit tests failed"
    fi
}

# ---------- main ----------

main() {
    echo ""
    echo "========================================="
    echo "  Bamboo MCP Server - Integration Tests  "
    echo "========================================="
    echo ""
    echo "  BAMBOO_URL:   ${BAMBOO_URL}"
    echo "  PROJECT_KEY:  ${PROJECT_KEY:-(not set)}"
    echo "  PLAN_KEY:     ${PLAN_KEY:-(not set)}"
    echo ""

    check_prereqs

    echo ""
    echo "-----------------------------------------"
    echo "  Build & Unit Tests"
    echo "-----------------------------------------"

    test_binary_builds
    test_unit_tests

    echo ""
    echo "-----------------------------------------"
    echo "  Bamboo REST API Tests"
    echo "-----------------------------------------"

    test_health_check
    test_server_info
    test_list_projects
    test_list_all_projects
    test_get_project
    test_list_plans
    test_search_plans
    test_get_plan
    test_list_plan_branches
    test_get_latest_result
    test_list_build_results
    test_get_build_queue
    test_list_deployment_projects
    test_get_deployment_project

    echo ""
    echo "========================================="
    echo "  Results"
    echo "========================================="
    echo -e "  ${GREEN}PASS: $PASS${NC}"
    echo -e "  ${RED}FAIL: $FAIL${NC}"
    echo -e "  ${YELLOW}SKIP: $SKIP${NC}"
    echo "========================================="
    echo ""

    if [[ $FAIL -gt 0 ]]; then
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi

    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
}

main "$@"
