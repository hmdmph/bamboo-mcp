# Architecture Overview

## Design Principles

The Bamboo MCP Server is designed with the following principles:

1. **Modularity**: Clear separation of concerns across packages
2. **Extensibility**: Easy to add new tools and features
3. **Maintainability**: Clean code structure with comprehensive error handling
4. **Security**: Secure storage of credentials with proper permissions

## Project Structure

```
bamboo-mcp/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, server init, tool registration
├── internal/
│   ├── bamboo/                  # Bamboo REST API client (read-only)
│   ├── config/                  # Environment configuration
│   ├── context/                 # Plan-naming context knowledge (context.yaml)
│   ├── logger/                  # Levelled logger with secret redaction (stderr)
│   ├── prompts/                 # MCP prompt templates
│   ├── security/                # Policy engine, sanitizer, handler middleware
│   ├── storage/                 # Bitbucket repo credential storage
│   ├── tools/                   # MCP tool handlers
│   └── validation/              # Startup connectivity/auth checks
├── examples/
│   └── context.yaml             # Worked example of a plan context config
├── scripts/                     # Integration test helpers
├── .env.example                 # Environment variable template
├── Dockerfile                   # Multi-stage container build
├── Makefile                     # Build automation
└── README.md                    # User documentation
```

## Component Responsibilities

### cmd/server/main.go
- Application entry point
- Loads configuration
- Initializes clients and storage
- Registers all MCP tools
- Starts the MCP server

### internal/config
- **config.go**: Loads and validates environment variables
- Provides centralized configuration management
- Validates required settings (BAMBOO_URL, BAMBOO_TOKEN)

### internal/bamboo
- **client.go**: HTTP client for Bamboo REST API
- Handles authentication with Bearer tokens
- Supports proxy configuration
- Implements all read-only Bamboo operations:
  - Server info and health checks
  - Project operations
  - Plan operations
  - Branch operations
  - Build operations
  - Deployment operations

### internal/storage
- **bitbucket.go**: Persistent storage for Bitbucket repository configurations
- JSON-based storage in `~/.bamboo-mcp/bitbucket-repos.json`
- Thread-safe operations with mutex locks
- Secure file permissions (0600)
- CRUD operations for repository configurations

### internal/security
- **config.go**: Loads `~/.bamboo-mcp/security.yaml`, seeding it with safe defaults
- **policy.go**: Evaluates every tool call — HTTP-method allowlist (GET-only floor),
  required/allowed/denied environments, result-count caps, and per-tool rate limits
- **sanitizer.go**: Scans API responses for prompt-injection patterns, redacts matches,
  labels content as untrusted, and truncates oversized responses
- **middleware.go**: `Wrap` composes the policy gate and sanitizer around a tool handler

### internal/context
- **context.go**: Loads plan-naming conventions from `~/.bamboo-mcp/context.yaml`
- Resolves human-friendly references to Bamboo plan keys
- Parses deployment environment names positionally against each plan type's
  declared `environment_format`
- Hot-reloadable at runtime via the `bamboo_reload_context` tool

### internal/prompts
- **prompts.go**: MCP prompt templates that chain tools into common workflows
  (deploy status, who-deployed-last, build investigation, health check)

### internal/validation
- **validate.go**: Startup checks for config completeness, URL format, Bamboo
  reachability, and token validity. Failures abort startup unless `SKIP_VALIDATION=true`

### internal/logger
- **logger.go**: Writes to **stderr** only — stdout is reserved for the stdio MCP
  protocol stream
- Applies regex redaction so bearer tokens and secret-shaped values never reach logs

### internal/tools
- **bamboo_tools.go**: MCP tool handlers for Bamboo operations
- **bitbucket_tools.go**: MCP tool handlers for Bitbucket storage
- Converts API responses to MCP tool results
- Handles parameter validation
- Provides user-friendly error messages

## Data Flow

### Bamboo Operations
```
MCP Client → MCP Server → Tool Handler → Bamboo Client → Bamboo API
                                                              ↓
MCP Client ← MCP Server ← Tool Handler ← Bamboo Client ← Response
```

### Bitbucket Storage Operations
```
MCP Client → MCP Server → Tool Handler → Storage → File System
                                                        ↓
MCP Client ← MCP Server ← Tool Handler ← Storage ← JSON File
```

## Security Considerations

0. **Read-only by default**: The policy engine enforces a GET-only floor on every
   tool; non-GET downstream calls are rejected unless explicitly allowlisted
1. **Token Storage**: Tokens stored with 0600 permissions (owner read/write only)
2. **Token Display**: Tokens masked when displayed (only last 4 chars shown)
3. **Configuration Location**: User home directory (`~/.bamboo-mcp/`)
4. **Environment Variables**: Sensitive data loaded from environment
5. **No Logging of Secrets**: Tokens never logged or exposed

## Extensibility

### Adding New Bamboo Tools

1. **Add API method** in `internal/bamboo/client.go`:
```go
func (c *Client) NewOperation() (map[string]interface{}, error) {
    data, err := c.doRequest("GET", "new/endpoint", nil)
    if err != nil {
        return nil, err
    }
    var result map[string]interface{}
    json.Unmarshal(data, &result)
    return result, nil
}
```

2. **Add tool handler** in `internal/tools/bamboo_tools.go`:
```go
func (t *BambooTools) NewOperation(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
    result, err := t.client.NewOperation()
    if err != nil {
        return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
    }
    content, _ := json.MarshalIndent(result, "", "  ")
    return mcp.NewToolResultText(string(content)), nil
}
```

3. **Register tool** in `cmd/server/main.go`:
```go
s.AddTool(mcp.NewTool("bamboo_new_operation",
    mcp.WithDescription("Description of the operation"),
    mcp.WithString("param", mcp.Required(), mcp.Description("Parameter description")),
), bambooTools.NewOperation)
```

### Adding New Storage Types

Follow the pattern in `internal/storage/bitbucket.go`:
1. Define struct for stored data
2. Implement CRUD operations
3. Use JSON for persistence
4. Ensure thread safety with mutexes
5. Create corresponding tool handlers

## Testing Strategy

- **Unit Tests**: Test individual components in isolation
- **Client Tests**: Validate HTTP client creation and error handling
- **Storage Tests**: Verify CRUD operations and file persistence
- **Integration Tests**: (Future) Test end-to-end workflows

## Performance Considerations

- HTTP client reuse with connection pooling
- 60-second timeout for API requests
- Thread-safe storage operations
- Efficient JSON marshaling/unmarshaling

## Future Enhancements

1. **Write Operations**: Add support for triggering builds and deployments
2. **Caching**: Implement caching for frequently accessed data
3. **Metrics**: Add monitoring and metrics collection
4. **Multi-Instance**: Support multiple Bamboo instances
5. **Build Logs**: Add build log retrieval functionality
6. **Webhooks**: Support for Bamboo webhooks
