package tools

import (
	"encoding/json"
	"fmt"

	"github.com/hmdmph/bamboo-mcp/internal/storage"
	"github.com/mark3labs/mcp-go/mcp"
)

type BitbucketTools struct {
	storage *storage.BitbucketStorage
}

func NewBitbucketTools(storage *storage.BitbucketStorage) *BitbucketTools {
	return &BitbucketTools{storage: storage}
}

// maskToken renders a token for display, revealing at most its last 4 characters.
// Short tokens are masked entirely rather than sliced (which would panic).
func maskToken(token string) string {
	if len(token) <= 4 {
		return "***"
	}
	return "***" + token[len(token)-4:]
}

func (t *BitbucketTools) AddRepo(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name, ok := arguments["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	url, ok := arguments["url"].(string)
	if !ok || url == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	username, ok := arguments["username"].(string)
	if !ok || username == "" {
		return mcp.NewToolResultError("username is required"), nil
	}

	token, ok := arguments["token"].(string)
	if !ok || token == "" {
		return mcp.NewToolResultError("token is required"), nil
	}

	repo := &storage.BitbucketRepo{
		URL:      url,
		Username: username,
		Token:    token,
	}

	if err := t.storage.AddRepo(name, repo); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add repository: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("✅ Successfully added Bitbucket repository '%s'", name)), nil
}

func (t *BitbucketTools) GetRepo(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name, ok := arguments["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	repo, err := t.storage.GetRepo(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repository: %v", err)), nil
	}

	maskedRepo := map[string]string{
		"name":     name,
		"url":      repo.URL,
		"username": repo.Username,
		"token":    maskToken(repo.Token),
	}

	content, err := json.MarshalIndent(maskedRepo, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BitbucketTools) ListRepos(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	repos := t.storage.ListRepos()

	maskedRepos := make(map[string]map[string]string)
	for name, repo := range repos {
		maskedRepos[name] = map[string]string{
			"url":      repo.URL,
			"username": repo.Username,
			"token":    maskToken(repo.Token),
		}
	}

	content, err := json.MarshalIndent(maskedRepos, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (t *BitbucketTools) DeleteRepo(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name, ok := arguments["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	if err := t.storage.DeleteRepo(name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete repository: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("✅ Successfully deleted Bitbucket repository '%s'", name)), nil
}
