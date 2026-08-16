package config

import (
	"fmt"
	"os"
	"strings"
)

// TransportMode defines how the MCP server communicates.
type TransportMode string

const (
	TransportStdio TransportMode = "stdio"
	TransportSSE   TransportMode = "sse"
)

type Config struct {
	BambooURL        string
	BambooToken      string
	BambooProxy      string
	BitbucketURL     string
	Verbose          bool
	ContextFilePath  string
	SecurityFilePath string
	Transport        TransportMode
	HTTPHost         string
	HTTPPort         string
	SkipValidation   bool
}

// HTTPAddr returns the host:port address for HTTP/SSE transport.
func (c *Config) HTTPAddr() string {
	host := c.HTTPHost
	if host == "" {
		host = "0.0.0.0"
	}
	port := c.HTTPPort
	if port == "" {
		port = "8080"
	}
	return host + ":" + port
}

// BaseURL returns the externally reachable base URL for the SSE server.
func (c *Config) BaseURL() string {
	host := os.Getenv("MCP_BASE_URL")
	if host != "" {
		return strings.TrimRight(host, "/")
	}
	port := c.HTTPPort
	if port == "" {
		port = "8080"
	}
	return "http://localhost:" + port
}

func Load() (*Config, error) {
	bambooURL := os.Getenv("BAMBOO_URL")
	if bambooURL == "" {
		return nil, fmt.Errorf("BAMBOO_URL environment variable is required")
	}

	bambooToken := os.Getenv("BAMBOO_TOKEN")
	if bambooToken == "" {
		return nil, fmt.Errorf("BAMBOO_TOKEN environment variable is required")
	}

	verbose := strings.ToLower(os.Getenv("VERBOSE"))
	isVerbose := verbose == "true" || verbose == "1" || verbose == "yes"

	skip := strings.ToLower(os.Getenv("SKIP_VALIDATION"))
	skipValidation := skip == "true" || skip == "1" || skip == "yes"

	transport := TransportMode(strings.ToLower(os.Getenv("MCP_TRANSPORT")))
	if transport == "" {
		transport = TransportStdio
	}
	if transport != TransportStdio && transport != TransportSSE {
		return nil, fmt.Errorf("invalid MCP_TRANSPORT '%s': must be 'stdio' or 'sse'", transport)
	}

	return &Config{
		BambooURL:        bambooURL,
		BambooToken:      bambooToken,
		BambooProxy:      os.Getenv("BAMBOO_PROXY"),
		BitbucketURL:     os.Getenv("BITBUCKET_URL"),
		Verbose:          isVerbose,
		ContextFilePath:  os.Getenv("CONTEXT_FILE"),
		SecurityFilePath: os.Getenv("SECURITY_FILE"),
		Transport:        transport,
		HTTPHost:         os.Getenv("MCP_HTTP_HOST"),
		HTTPPort:         os.Getenv("MCP_HTTP_PORT"),
		SkipValidation:   skipValidation,
	}, nil
}
