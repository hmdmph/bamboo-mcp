package validation

import (
	"fmt"
	"strings"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	"github.com/hmdmph/bamboo-mcp/internal/config"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
)

// Result holds the outcome of a single validation check.
type Result struct {
	Name    string
	OK      bool
	Message string
}

// Report summarises all validation results.
type Report struct {
	Results []Result
	Passed  int
	Failed  int
}

func (r *Report) add(name string, ok bool, msg string) {
	r.Results = append(r.Results, Result{Name: name, OK: ok, Message: msg})
	if ok {
		r.Passed++
	} else {
		r.Failed++
	}
}

// Run performs all startup validation checks and returns a Report.
// If any critical check fails and cfg.SkipValidation is false, it returns an error.
func Run(cfg *config.Config, client *bamboo.Client) (*Report, error) {
	report := &Report{}

	// 1. Config completeness
	if cfg.BambooURL != "" && cfg.BambooToken != "" {
		report.add("config", true, fmt.Sprintf("BAMBOO_URL=%s, token present", cfg.BambooURL))
	} else {
		report.add("config", false, "BAMBOO_URL or BAMBOO_TOKEN is missing")
	}

	// 2. Bamboo URL format
	if strings.HasPrefix(cfg.BambooURL, "http://") || strings.HasPrefix(cfg.BambooURL, "https://") {
		report.add("url_format", true, "Bamboo URL has valid scheme")
	} else {
		report.add("url_format", false, "BAMBOO_URL must start with http:// or https://")
	}

	// 3. Bamboo server reachability (health check)
	ok, err := client.HealthCheck()
	if !ok || err != nil {
		report.add("bamboo_connectivity", false, fmt.Sprintf("Cannot reach Bamboo: %v", err))
	} else {
		report.add("bamboo_connectivity", true, "Bamboo server is reachable")
	}

	// 4. Token validity + server info
	info, err := client.GetServerInfo()
	if err != nil {
		report.add("bamboo_auth", false, fmt.Sprintf("Token validation failed: %v", err))
	} else {
		version := ""
		if v, ok := info["version"].(string); ok {
			version = v
		}
		report.add("bamboo_auth", true, fmt.Sprintf("Bamboo token is valid (server version: %s)", version))
	}

	// 5. Transport mode
	transport := string(cfg.Transport)
	if transport == "sse" {
		report.add("transport", true, fmt.Sprintf("SSE/HTTP transport on %s", cfg.HTTPAddr()))
	} else {
		report.add("transport", true, "stdio transport")
	}

	// Print summary
	for _, r := range report.Results {
		icon := "✓"
		if !r.OK {
			icon = "✗"
		}
		logger.Info("Validation [%s] %s: %s", icon, r.Name, r.Message)
	}

	logger.Info("Validation complete: %d passed, %d failed", report.Passed, report.Failed)

	if report.Failed > 0 && !cfg.SkipValidation {
		var failures []string
		for _, r := range report.Results {
			if !r.OK {
				failures = append(failures, r.Name)
			}
		}
		return report, fmt.Errorf("validation failed (%s) — set SKIP_VALIDATION=true to start anyway", strings.Join(failures, ", "))
	}

	return report, nil
}
