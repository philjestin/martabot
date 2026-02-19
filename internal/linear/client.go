package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const graphqlEndpoint = "https://api.linear.app/graphql"

type Client struct {
	apiKey string
	http   *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{},
	}
}

const issueCreateMutation = `
mutation IssueCreate($title: String!, $description: String!, $teamId: String!, $priority: Int!) {
  issueCreate(input: { title: $title, description: $description, teamId: $teamId, priority: $priority }) {
    success
    issue {
      id
      identifier
      url
    }
  }
}`

func (c *Client) CreateIssue(input IssueCreateInput) (*IssueCreateResponse, error) {
	body := map[string]any{
		"query": issueCreateMutation,
		"variables": map[string]any{
			"title":       input.Title,
			"description": input.Description,
			"teamId":      input.TeamID,
			"priority":    input.Priority,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", graphqlEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result IssueCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear GraphQL error: %s", result.Errors[0].Message)
	}

	if !result.Data.IssueCreate.Success {
		return nil, fmt.Errorf("linear issue creation reported failure")
	}

	return &result, nil
}
