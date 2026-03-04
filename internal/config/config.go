package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	SlackBotToken  string
	SlackChannelID string
	SeenEmoji      string
	IgnoreEmoji    string
	PostReply      bool

	GCPProject  string
	GCPRegion   string
	ClaudeModel string

	LinearAPIKey    string
	LinearTeamID    string
	LinearProjectID string
	LinearLabelIDs  []string
	LinearAssigneeID string
	LinearStateID   string
	LinearDocumentID string

	InsightsChannelID string

	DryRun  bool
	Verbose bool
}

func Load(dryRun, verbose bool) (*Config, error) {
	cfg := &Config{
		SlackBotToken:   os.Getenv("SLACK_BOT_TOKEN"),
		SlackChannelID:  os.Getenv("SLACK_CHANNEL_ID"),
		SeenEmoji:       os.Getenv("SEEN_EMOJI"),
		IgnoreEmoji:     os.Getenv("IGNORE_EMOJI"),
		PostReply:       true,
		GCPProject:  os.Getenv("GCP_PROJECT"),
		GCPRegion:   os.Getenv("GCP_REGION"),
		ClaudeModel: os.Getenv("CLAUDE_MODEL"),
		LinearAPIKey:     os.Getenv("LINEAR_API_KEY"),
		LinearTeamID:     os.Getenv("LINEAR_TEAM_ID"),
		LinearProjectID:  os.Getenv("LINEAR_PROJECT_ID"),
		LinearAssigneeID:  os.Getenv("LINEAR_ASSIGNEE_ID"),
		LinearStateID:     os.Getenv("LINEAR_STATE_ID"),
		LinearDocumentID:  os.Getenv("LINEAR_DOCUMENT_ID"),
		InsightsChannelID: os.Getenv("INSIGHTS_CHANNEL_ID"),
		DryRun:            dryRun,
		Verbose:         verbose,
	}

	if cfg.SeenEmoji == "" {
		cfg.SeenEmoji = "eyes"
	}
	if cfg.IgnoreEmoji == "" {
		cfg.IgnoreEmoji = "no_entry_sign"
	}
	if cfg.ClaudeModel == "" {
		cfg.ClaudeModel = "claude-sonnet-4-5"
	}
	if pr := os.Getenv("POST_REPLY"); pr != "" {
		cfg.PostReply = strings.EqualFold(pr, "true")
	}
	if labelIDs := os.Getenv("LINEAR_LABEL_IDS"); labelIDs != "" {
		for _, id := range strings.Split(labelIDs, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				cfg.LinearLabelIDs = append(cfg.LinearLabelIDs, trimmed)
			}
		}
	}

	var missing []string
	if cfg.SlackBotToken == "" {
		missing = append(missing, "SLACK_BOT_TOKEN")
	}
	if cfg.SlackChannelID == "" {
		missing = append(missing, "SLACK_CHANNEL_ID")
	}
	if cfg.GCPProject == "" {
		missing = append(missing, "GCP_PROJECT")
	}
	if cfg.GCPRegion == "" {
		missing = append(missing, "GCP_REGION")
	}
	if cfg.LinearAPIKey == "" {
		missing = append(missing, "LINEAR_API_KEY")
	}
	if cfg.LinearTeamID == "" {
		missing = append(missing, "LINEAR_TEAM_ID")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
