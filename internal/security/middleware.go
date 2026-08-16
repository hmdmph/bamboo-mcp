package security

import (
	"fmt"

	"github.com/hmdmph/bamboo-mcp/internal/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Wrap returns a new ToolHandlerFunc that applies policy evaluation and response
// sanitization around the original handler.  Every registered tool should be
// wrapped so that the security layer is enforced uniformly.
//
// Usage in main.go:
//
//	s.AddTool(mcp.NewTool("bamboo_foo", ...), sec.Wrap("bamboo_foo", "GET", handler))
func (e *PolicyEngine) Wrap(
	toolName string,
	httpMethod string,
	handler server.ToolHandlerFunc,
	sanitizer *Sanitizer,
) server.ToolHandlerFunc {
	return func(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
		// ── 1. Policy gate ────────────────────────────────────────────────────
		result := e.Evaluate(PolicyRequest{
			Tool:       toolName,
			Arguments:  arguments,
			HTTPMethod: httpMethod,
		})
		if !result.Allowed {
			logger.Info("Security: DENIED tool=%s reason=%s", toolName, result.Reason)
			return mcp.NewToolResultError(
				fmt.Sprintf("[SECURITY_POLICY_DENIED] %s", result.Reason),
			), nil
		}

		// ── 2. Execute handler ────────────────────────────────────────────────
		toolResult, err := handler(arguments)
		if err != nil {
			return nil, err
		}

		// ── 3. Sanitize response ──────────────────────────────────────────────
		return sanitizer.SanitizeResult(toolResult), nil
	}
}
