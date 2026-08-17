package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hmdmph/bamboo-mcp/internal/bamboo"
	"github.com/hmdmph/bamboo-mcp/internal/config"
	pctx "github.com/hmdmph/bamboo-mcp/internal/context"
	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/hmdmph/bamboo-mcp/internal/prompts"
	"github.com/hmdmph/bamboo-mcp/internal/security"
	"github.com/hmdmph/bamboo-mcp/internal/storage"
	"github.com/hmdmph/bamboo-mcp/internal/tools"
	"github.com/hmdmph/bamboo-mcp/internal/validation"
	"github.com/mark3labs/mcp-go/server"
)

// version is the server version, overridden at build time with
// -ldflags "-X main.version=v1.2.3". Release builds set it from the git tag.
var version = "dev"

func main() {
	listTools := flag.Bool("list-tools", false,
		"Print the tool catalog as an MCP tools/list payload and exit. Needs no Bamboo connection.")
	showVersion := flag.Bool("version", false, "Print the server version and exit.")
	validateOnly := flag.Bool("validate-only", false,
		"Run the startup checks, print the report and exit without serving.")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// --list-tools runs before any config or network work so that anything
	// holding the binary — a registry scanner, CI, a curious user — can read the
	// tool surface without credentials or a reachable Bamboo.
	if *listTools {
		payload, err := tools.CatalogJSON()
		if err != nil {
			log.Fatalf("Failed to render tool catalog: %v", err)
		}
		if _, err := os.Stdout.Write(append(payload, '\n')); err != nil {
			log.Fatalf("Failed to write tool catalog: %v", err)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logger.SetVerbose(cfg.Verbose)
	logger.Info("Starting Bamboo MCP Server %s", version)
	logger.Info("Bamboo URL: %s", cfg.BambooURL)
	logger.Info("Transport: %s", cfg.Transport)

	bambooClient, err := bamboo.NewClient(cfg.BambooURL, cfg.BambooToken, cfg.BambooProxy)
	if err != nil {
		log.Fatalf("Failed to create Bamboo client: %v", err)
	}

	// Startup validation
	report, err := validation.Run(cfg, bambooClient)
	if *validateOnly {
		printValidationReport(report)
		if err != nil {
			log.Fatalf("Startup validation failed: %v", err)
		}
		return
	}
	if err != nil {
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
		version,
		server.WithPromptCapabilities(false),
	)

	registerTools(s, tools.Handlers(bambooTools, contextTools, bitbucketTools), policy, sanitizer)
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

// printValidationReport writes the startup checks to stdout for --validate-only.
// A nil report means Run bailed out before recording anything.
func printValidationReport(report *validation.Report) {
	if report == nil {
		fmt.Println("No validation results were produced.")
		return
	}

	for _, result := range report.Results {
		mark := "FAIL"
		if result.OK {
			mark = "ok"
		}
		fmt.Printf("[%-4s] %-20s %s\n", mark, result.Name, result.Message)
	}
	fmt.Printf("\n%d passed, %d failed\n", report.Passed, report.Failed)
}

// registerTools wires each declaration in the tool catalog to its handler,
// routed through the security policy engine. A declaration without a handler is
// a programming error, so it aborts startup rather than silently exposing a tool
// that cannot be called.
func registerTools(s *server.MCPServer, handlers map[string]tools.Handler, p *security.PolicyEngine, san *security.Sanitizer) {
	catalog := tools.Catalog()

	for _, def := range catalog {
		h, ok := handlers[def.Name]
		if !ok {
			log.Fatalf("Tool %q is declared in the catalog but has no handler", def.Name)
		}
		// Every tool is a read: the policy engine enforces the GET-only floor.
		s.AddTool(def, p.Wrap(def.Name, "GET", server.ToolHandlerFunc(h), san))
	}

	if len(handlers) != len(catalog) {
		log.Fatalf("Tool catalog declares %d tools but %d handlers are registered", len(catalog), len(handlers))
	}

	logger.Info("Registered %d tools", len(catalog))
}
