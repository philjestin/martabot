package linear

type IssueCreateInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TeamID      string `json:"teamId"`
	Priority    int    `json:"priority"`
	ProjectID   string `json:"projectId,omitempty"`
}

type IssueCreateResponse struct {
	Data struct {
		IssueCreate struct {
			Success bool `json:"success"`
			Issue   struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				URL        string `json:"url"`
			} `json:"issue"`
		} `json:"issueCreate"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}
