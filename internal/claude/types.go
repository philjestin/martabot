package claude

type TicketSuggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type InsightSummary struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type EvaluationResult struct {
	Category string             `json:"category"`
	Reason   string             `json:"reason"`
	Tickets  []TicketSuggestion `json:"tickets,omitempty"`
	Insight  *InsightSummary    `json:"insight,omitempty"`
}
