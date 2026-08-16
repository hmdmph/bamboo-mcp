package security

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PolicyRequest contains all context needed to evaluate a policy decision.
type PolicyRequest struct {
	Tool       string
	Arguments  map[string]interface{}
	HTTPMethod string // the downstream HTTP verb this tool uses (default "GET")
}

// PolicyResult is the outcome of a policy evaluation.
type PolicyResult struct {
	Allowed bool
	Reason  string
}

// toolBucket is a simple per-minute sliding counter for rate limiting.
type toolBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

func (b *toolBucket) allow(limit int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(time.Minute)
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

// PolicyEngine evaluates security policy for every tool call.
type PolicyEngine struct {
	cfg     *SecurityConfig
	buckets sync.Map // map[string]*toolBucket
}

// NewPolicyEngine creates a PolicyEngine from a SecurityConfig.
func NewPolicyEngine(cfg *SecurityConfig) *PolicyEngine {
	return &PolicyEngine{cfg: cfg}
}

// Evaluate checks all security rules for the given request and returns whether the call is permitted.
func (e *PolicyEngine) Evaluate(req PolicyRequest) PolicyResult {
	policy := e.toolPolicy(req.Tool)

	// 1. Disabled tool
	if policy.Disabled {
		return deny("tool '%s' is disabled by security policy", req.Tool)
	}

	// 2. HTTP method allowlist — always enforce GET-only floor
	method := strings.ToUpper(req.HTTPMethod)
	if method == "" {
		method = "GET"
	}
	allowed := policy.AllowedMethods
	if len(allowed) == 0 {
		allowed = e.cfg.DefaultAllowedMethods
	}
	if len(allowed) == 0 {
		allowed = []string{"GET"}
	}
	if !containsStr(allowed, method) {
		return deny("HTTP method '%s' is not permitted for tool '%s' (allowed: %s) — possible write attempt blocked",
			method, req.Tool, strings.Join(allowed, ","))
	}

	// 3. Explicit environment requirement
	if e.requiresEnv(req.Tool, policy) {
		env := extractStringArg(req.Arguments, "environment", "env")
		if env == "" {
			return deny("tool '%s' requires an explicit 'environment' argument — "+
				"environment is never inferred from context or prompt text", req.Tool)
		}
		env = strings.ToLower(strings.TrimSpace(env))

		// 4. Denied environments
		for _, d := range policy.DeniedEnvironments {
			if strings.ToLower(d) == env {
				return deny("environment '%s' is denied for tool '%s' by security policy", env, req.Tool)
			}
		}

		// 5. Allowlist environments
		if len(policy.AllowedEnvironments) > 0 && !containsStr(policy.AllowedEnvironments, env) {
			return deny("environment '%s' is not in the allowed list for tool '%s' (allowed: %s)",
				env, req.Tool, strings.Join(policy.AllowedEnvironments, ","))
		}
	}

	// 6. Max results guard — checks maxResults, max-results, limit arguments
	if policy.MaxResults > 0 {
		if n := extractMaxResults(req.Arguments); n > policy.MaxResults {
			return deny("requested %d results exceeds the maximum of %d for tool '%s'",
				n, policy.MaxResults, req.Tool)
		}
	}

	// 7. Rate limiting
	limit := policy.RateLimitPerMinute
	if limit == 0 {
		limit = e.cfg.DefaultRateLimitPerMinute
	}
	if limit > 0 && !e.bucket(req.Tool).allow(limit) {
		return deny("rate limit exceeded for tool '%s' (%d calls/min)", req.Tool, limit)
	}

	return PolicyResult{Allowed: true}
}

// toolPolicy returns the merged policy for a tool (defaults + tool-specific overrides).
func (e *PolicyEngine) toolPolicy(tool string) ToolPolicy {
	if p, ok := e.cfg.ToolPolicies[tool]; ok {
		return p
	}
	return ToolPolicy{}
}

func (e *PolicyEngine) requiresEnv(tool string, policy ToolPolicy) bool {
	if policy.RequireEnvironment || e.cfg.RequireExplicitEnvironment {
		return true
	}
	return containsStr(e.cfg.EnvSensitiveTools, tool)
}

func (e *PolicyEngine) bucket(tool string) *toolBucket {
	v, _ := e.buckets.LoadOrStore(tool, &toolBucket{resetAt: time.Now().Add(time.Minute)})
	return v.(*toolBucket)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func deny(format string, args ...interface{}) PolicyResult {
	return PolicyResult{Allowed: false, Reason: fmt.Sprintf(format, args...)}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// extractStringArg looks for the first matching key in arguments, case-insensitively.
func extractStringArg(args map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// extractMaxResults finds numeric result-count arguments and returns the largest.
func extractMaxResults(args map[string]interface{}) int {
	keys := []string{"maxResults", "max_results", "max-results", "limit", "maxResult"}
	max := 0
	for _, k := range keys {
		if v, ok := args[k]; ok {
			switch n := v.(type) {
			case float64:
				if int(n) > max {
					max = int(n)
				}
			case int:
				if n > max {
					max = n
				}
			}
		}
	}
	return max
}
