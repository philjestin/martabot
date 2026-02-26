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

const fileUploadMutation = `
mutation FileUpload($contentType: String!, $filename: String!, $size: Int!) {
  fileUpload(contentType: $contentType, filename: $filename, size: $size) {
    uploadFile {
      uploadUrl
      assetUrl
      headers {
        key
        value
      }
    }
  }
}`

const issueCreateMutation = `
mutation IssueCreate($title: String!, $description: String!, $teamId: String!) {
  issueCreate(input: { title: $title, description: $description, teamId: $teamId }) {
    success
    issue {
      id
      identifier
      url
    }
  }
}`

const issueCreateWithProjectMutation = `
mutation IssueCreate($title: String!, $description: String!, $teamId: String!, $projectId: String!) {
  issueCreate(input: { title: $title, description: $description, teamId: $teamId, projectId: $projectId }) {
    success
    issue {
      id
      identifier
      url
    }
  }
}`

func (c *Client) CreateIssue(input IssueCreateInput) (*IssueCreateResponse, error) {
	query := issueCreateMutation
	vars := map[string]any{
		"title":       input.Title,
		"description": input.Description,
		"teamId":      input.TeamID,
	}
	if input.ProjectID != "" {
		query = issueCreateWithProjectMutation
		vars["projectId"] = input.ProjectID
	}

	body := map[string]any{
		"query":     query,
		"variables": vars,
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

func (c *Client) UploadFile(filename, contentType string, data []byte) (string, error) {
	body := map[string]any{
		"query": fileUploadMutation,
		"variables": map[string]any{
			"contentType": contentType,
			"filename":    filename,
			"size":        len(data),
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling file upload request: %w", err)
	}

	req, err := http.NewRequest("POST", graphqlEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating file upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing file upload request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading file upload response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("file upload API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result FileUploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing file upload response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("file upload GraphQL error: %s", result.Errors[0].Message)
	}

	upload := result.Data.FileUpload.UploadFile

	// PUT the file to the presigned upload URL
	putReq, err := http.NewRequest("PUT", upload.UploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating PUT request: %w", err)
	}
	putReq.Header.Set("Content-Type", contentType)
	putReq.Header.Set("Cache-Control", "public, max-age=31536000")
	for _, h := range upload.Headers {
		putReq.Header.Set(h.Key, h.Value)
	}

	putResp, err := c.http.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("uploading file: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		putBody, _ := io.ReadAll(putResp.Body)
		return "", fmt.Errorf("file PUT returned %d: %s", putResp.StatusCode, string(putBody))
	}

	return upload.AssetURL, nil
}
