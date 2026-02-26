package claude

type TicketSuggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type EvaluationResult struct {
	ShouldCreateTicket bool              `json:"should_create_ticket"`
	Reason             string            `json:"reason"`
	Ticket             *TicketSuggestion `json:"ticket,omitempty"`
}
