package bamboo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hmdmph/bamboo-mcp/internal/logger"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token, proxyURL string) (*Client, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
	}

	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: client,
	}, nil
}

func (c *Client) GetBaseURL() string {
	return c.baseURL
}

func (c *Client) doRequest(method, path string, params url.Values) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/rest/api/latest/%s", c.baseURL, path)
	if params != nil {
		reqURL = fmt.Sprintf("%s?%s", reqURL, params.Encode())
	}

	logger.Info("%s %s", method, reqURL)

	req, err := http.NewRequest(method, reqURL, nil)
	if err != nil {
		logger.Error("Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token) // token never written to logs
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bamboo-mcp/1.0")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		logger.Error("Request failed after %s: %v", duration, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	logger.Info("Response: %d (%s)", resp.StatusCode, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Debug("Response body length: %d bytes", len(body))

	if resp.StatusCode >= 400 {
		logger.Error("API error (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) GetServerInfo() (map[string]interface{}, error) {
	data, err := c.doRequest("GET", "info", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) HealthCheck() (bool, error) {
	_, err := c.doRequest("GET", "info", nil)
	return err == nil, err
}

func (c *Client) ListProjects(maxResult int, startIndex int) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("expand", "projects.project")
	if maxResult > 0 {
		params.Set("max-result", fmt.Sprintf("%d", maxResult))
	}
	if startIndex > 0 {
		params.Set("start-index", fmt.Sprintf("%d", startIndex))
	}

	data, err := c.doRequest("GET", "project", params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) ListAllProjects() ([]map[string]interface{}, error) {
	var allProjects []map[string]interface{}
	startIndex := 0
	pageSize := 25

	for {
		result, err := c.ListProjects(pageSize, startIndex)
		if err != nil {
			return nil, err
		}

		projects, ok := result["projects"].(map[string]interface{})
		if !ok {
			break
		}

		projectList, ok := projects["project"].([]interface{})
		if !ok {
			break
		}

		for _, p := range projectList {
			if proj, ok := p.(map[string]interface{}); ok {
				allProjects = append(allProjects, proj)
			}
		}

		totalSize := 0
		if s, ok := projects["size"].(float64); ok {
			totalSize = int(s)
		}

		startIndex += pageSize
		if startIndex >= totalSize {
			break
		}
	}

	return allProjects, nil
}

func (c *Client) GetProject(projectKey string) (map[string]interface{}, error) {
	path := fmt.Sprintf("project/%s", projectKey)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) ListPlans() (map[string]interface{}, error) {
	data, err := c.doRequest("GET", "plan", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) GetPlan(planKey string) (map[string]interface{}, error) {
	path := fmt.Sprintf("plan/%s", planKey)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) SearchPlans(searchTerm string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("searchTerm", searchTerm)

	data, err := c.doRequest("GET", "search/plans", params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) ListPlanBranches(planKey string) (map[string]interface{}, error) {
	path := fmt.Sprintf("plan/%s/branch", planKey)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) GetPlanBranch(planKey, branchName string) (map[string]interface{}, error) {
	path := fmt.Sprintf("plan/%s/branch/%s", planKey, branchName)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) GetLatestResult(planKey string) (map[string]interface{}, error) {
	path := fmt.Sprintf("result/%s/latest", planKey)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) ListBuildResults(planKey string, maxResults int) (map[string]interface{}, error) {
	params := url.Values{}
	if maxResults > 0 {
		params.Set("max-results", fmt.Sprintf("%d", maxResults))
	}

	path := fmt.Sprintf("result/%s", planKey)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) GetBuildResult(buildKey string) (map[string]interface{}, error) {
	path := fmt.Sprintf("result/%s", buildKey)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetBuildResultExpanded returns a build result with expanded details (changes, artifacts, metadata, comments, labels, jiraIssues, stages).
func (c *Client) GetBuildResultExpanded(buildKey string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("expand", "changes,metadata,artifacts,comments,labels,jiraIssues,stages.stage.results.result")

	path := fmt.Sprintf("result/%s", buildKey)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetBuildRepositories returns VCS revisions (repositories + commits) for a specific build result.
func (c *Client) GetBuildRepositories(buildKey string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("expand", "vcsRevisions.vcsRevision.repositoryData")

	path := fmt.Sprintf("result/%s", buildKey)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetPlanRepositories returns the repositories linked to a specific plan.
func (c *Client) GetPlanRepositories(planKey string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("expand", "vcsLocations")

	path := fmt.Sprintf("plan/%s", planKey)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) GetBuildQueue() (map[string]interface{}, error) {
	data, err := c.doRequest("GET", "queue", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *Client) ListDeploymentProjects(maxResult int) ([]interface{}, error) {
	logger.Info("Fetching deployment projects (max: %d)", maxResult)
	data, err := c.doRequest("GET", "deploy/project/all", nil)
	if err != nil {
		return nil, err
	}

	var result []interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		var singleResult map[string]interface{}
		if err2 := json.Unmarshal(data, &singleResult); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		result = []interface{}{singleResult}
	}

	if maxResult > 0 && len(result) > maxResult {
		result = result[:maxResult]
	}

	return result, nil
}

func (c *Client) ListDeploymentProjectsForPlan(planKey string) ([]interface{}, error) {
	params := url.Values{}
	params.Set("planKey", planKey)

	data, err := c.doRequest("GET", "deploy/project/forPlan", params)
	if err != nil {
		return nil, err
	}

	var result []interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		var singleResult map[string]interface{}
		if err2 := json.Unmarshal(data, &singleResult); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return []interface{}{singleResult}, nil
	}

	return result, nil
}

func (c *Client) GetDeploymentProject(projectID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("deploy/project/%s", projectID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetDeploymentEnvironmentResults returns deployment results (history) for a specific environment.
// GET /rest/api/latest/deploy/environment/{environmentId}/results
func (c *Client) GetDeploymentEnvironmentResults(environmentID string, maxResults int) (map[string]interface{}, error) {
	params := url.Values{}
	if maxResults > 0 {
		params.Set("max-results", fmt.Sprintf("%d", maxResults))
	}

	path := fmt.Sprintf("deploy/environment/%s/results", environmentID)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetDeploymentResult returns details for a specific deployment result (who, when, status, etc.).
// GET /rest/api/latest/deploy/result/{deploymentResultId}
func (c *Client) GetDeploymentResult(resultID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("deploy/result/%s", resultID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// ListDeploymentVersions lists all versions (releases) for a deployment project.
// GET /rest/api/latest/deploy/project/{projectId}/versions
func (c *Client) ListDeploymentVersions(projectID string, maxResults int) (map[string]interface{}, error) {
	params := url.Values{}
	if maxResults > 0 {
		params.Set("max-results", fmt.Sprintf("%d", maxResults))
	}

	path := fmt.Sprintf("deploy/project/%s/versions", projectID)
	data, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetDeploymentVersion returns details of a specific deployment version/release.
// GET /rest/api/latest/deploy/version/{versionId}
func (c *Client) GetDeploymentVersion(versionID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("deploy/version/%s", versionID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// GetDeploymentVersionStatus returns the status of a version across all environments.
// GET /rest/api/latest/deploy/version/{versionId}/status
func (c *Client) GetDeploymentVersionStatus(versionID string) ([]interface{}, error) {
	path := fmt.Sprintf("deploy/version/%s/status", versionID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result []interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		var singleResult map[string]interface{}
		if err2 := json.Unmarshal(data, &singleResult); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return []interface{}{singleResult}, nil
	}

	return result, nil
}
