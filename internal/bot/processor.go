package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pmiddleton/martabot/internal/claude"
	"github.com/pmiddleton/martabot/internal/config"
	"github.com/pmiddleton/martabot/internal/linear"
	mslack "github.com/pmiddleton/martabot/internal/slack"
	"github.com/slack-go/slack"
)

const maxThreadChars = 8000

type Stats struct {
	Total    int
	Seen     int
	Created  int
	Insights int
	Skipped  int
	Errors   int
}

type Processor struct {
	cfg    *config.Config
	slack  *mslack.Client
	claude *claude.Client
	linear *linear.Client
}

func NewProcessor(cfg *config.Config, sc *mslack.Client, cc *claude.Client, lc *linear.Client) *Processor {
	return &Processor{
		cfg:    cfg,
		slack:  sc,
		claude: cc,
		linear: lc,
	}
}

func (p *Processor) Run(ctx context.Context) error {
	channels := []string{p.cfg.SlackChannelID}
	if p.cfg.InsightsChannelID != "" && p.cfg.InsightsChannelID != p.cfg.SlackChannelID {
		channels = append(channels, p.cfg.InsightsChannelID)
	}

	stats := Stats{}

	for _, channelID := range channels {
		log.Printf("Fetching messages from Slack channel %s...", channelID)

		messages, err := p.slack.FetchThreadParents(channelID)
		if err != nil {
			return fmt.Errorf("fetching messages from channel %s: %w", channelID, err)
		}

		stats.Total += len(messages)
		log.Printf("Found %d top-level messages in channel %s", len(messages), channelID)

		for _, msg := range messages {
			if p.slack.IsBotMessage(msg) {
				if p.cfg.Verbose {
					log.Printf("  Skipping bot message ts=%s", msg.Timestamp)
				}
				stats.Seen++
				continue
			}

			if p.slack.HasSeenReaction(msg) {
				if p.cfg.Verbose {
					log.Printf("  Already seen ts=%s", msg.Timestamp)
				}
				stats.Seen++
				continue
			}

			if p.slack.HasIgnoreReaction(msg) {
				if p.cfg.Verbose {
					log.Printf("  Ignored ts=%s", msg.Timestamp)
				}
				stats.Seen++
				continue
			}

			if err := p.processThread(ctx, channelID, msg, &stats); err != nil {
				log.Printf("  Error processing thread ts=%s: %v", msg.Timestamp, err)
				stats.Errors++
				continue
			}

			// Simple rate limiter: ~1 req/sec for Slack
			time.Sleep(1 * time.Second)
		}
	}

	log.Printf("Done. Total=%d Seen=%d Created=%d Insights=%d Skipped=%d Errors=%d",
		stats.Total, stats.Seen, stats.Created, stats.Insights, stats.Skipped, stats.Errors)

	return nil
}

func (p *Processor) processThread(ctx context.Context, channelID string, msg slack.Message, stats *Stats) error {
	log.Printf("  Processing thread ts=%s (user=%s, replies=%d)", msg.Timestamp, msg.User, msg.ReplyCount)

	thread := &mslack.Thread{
		Parent: mslack.Message{
			User:      msg.User,
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
			Files:     mslack.ExtractImageFiles(msg.Files),
		},
	}

	if msg.ReplyCount > 0 {
		replies, err := p.slack.FetchThreadReplies(channelID, msg.Timestamp)
		if err != nil {
			return err
		}
		for _, r := range replies {
			if p.slack.IsBotMessage(r) {
				continue
			}
			thread.Replies = append(thread.Replies, mslack.Message{
				User:      r.User,
				Text:      r.Text,
				Timestamp: r.Timestamp,
				Files:     mslack.ExtractImageFiles(r.Files),
			})
		}
	}

	// Resolve user IDs to display names
	thread.Parent.User = p.slack.ResolveUserName(thread.Parent.User)
	for i := range thread.Replies {
		thread.Replies[i].User = p.slack.ResolveUserName(thread.Replies[i].User)
	}

	threadText := thread.FormatForClaude(maxThreadChars)

	if p.cfg.Verbose {
		log.Printf("  Thread text:\n%s", threadText)
	}

	log.Println("  Evaluating with Claude...")
	result, err := p.claude.Evaluate(ctx, threadText)
	if err != nil {
		return fmt.Errorf("claude evaluation: %w", err)
	}

	log.Printf("  Claude decision: category=%q reason=%q", result.Category, result.Reason)

	switch result.Category {
	case "feedback":
		return p.handleFeedback(channelID, msg, thread, result, stats)
	case "insight":
		return p.handleInsight(channelID, msg, result, stats)
	default:
		// "skip" or any unrecognized category
		stats.Skipped++
		if !p.cfg.DryRun {
			if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}
}

func (p *Processor) handleFeedback(channelID string, msg slack.Message, thread *mslack.Thread, result *claude.EvaluationResult, stats *Stats) error {
	if len(result.Tickets) == 0 {
		stats.Skipped++
		if !p.cfg.DryRun {
			if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}

	// Get Slack thread permalink to include in the ticket
	slackLink, err := p.slack.GetPermalink(channelID, msg.Timestamp)
	if err != nil {
		log.Printf("  Warning: failed to get Slack permalink: %v", err)
		slackLink = ""
	}

	// Collect and upload images once (shared across all tickets from this thread)
	images := collectThreadImages(thread)
	var imageMarkdown string
	if !p.cfg.DryRun && len(images) > 0 {
		imageMarkdown = p.uploadImagesToLinear(images)
	}

	log.Printf("  Claude suggested %d ticket(s) from this thread", len(result.Tickets))

	var createdTickets []string
	for _, ticket := range result.Tickets {
		description := ticket.Description
		if slackLink != "" {
			description = fmt.Sprintf("%s\n\n---\n[Slack thread](%s)", description, slackLink)
		}

		if p.cfg.DryRun {
			log.Printf("  [DRY RUN] Would create ticket: %s", ticket.Title)
			log.Printf("  [DRY RUN] Description: %s", description)
			if len(images) > 0 {
				log.Printf("  [DRY RUN] Would upload %d image(s)", len(images))
			}
			stats.Created++
			continue
		}

		if imageMarkdown != "" {
			description = fmt.Sprintf("%s\n\n### Attachments\n%s", description, imageMarkdown)
		}

		log.Printf("  Creating Linear ticket: %s", ticket.Title)
		issueResp, err := p.linear.CreateIssue(linear.IssueCreateInput{
			Title:       ticket.Title,
			Description: description,
			TeamID:      p.cfg.LinearTeamID,
			ProjectID:   p.cfg.LinearProjectID,
			LabelIDs:    p.cfg.LinearLabelIDs,
			AssigneeID:  p.cfg.LinearAssigneeID,
			StateID:     p.cfg.LinearStateID,
		})
		if err != nil {
			log.Printf("  Error creating ticket %q: %v", ticket.Title, err)
			stats.Errors++
			continue
		}

		issue := issueResp.Data.IssueCreate.Issue
		log.Printf("  Created ticket %s: %s", issue.Identifier, issue.URL)
		createdTickets = append(createdTickets, fmt.Sprintf("%s: %s", issue.Identifier, issue.URL))
		stats.Created++
	}

	if p.cfg.DryRun {
		return nil
	}

	if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
		log.Printf("  Warning: failed to add reaction: %v", err)
	}

	if p.cfg.PostReply && len(createdTickets) > 0 {
		replyText := fmt.Sprintf("Created %d ticket(s):\n%s", len(createdTickets), strings.Join(createdTickets, "\n"))
		if err := p.slack.PostThreadReply(channelID, msg.Timestamp, replyText); err != nil {
			log.Printf("  Warning: failed to post thread reply: %v", err)
		}
	}

	return nil
}

func (p *Processor) handleInsight(channelID string, msg slack.Message, result *claude.EvaluationResult, stats *Stats) error {
	if result.Insight == nil {
		stats.Skipped++
		if !p.cfg.DryRun {
			if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}

	if p.cfg.LinearDocumentID == "" {
		log.Printf("  Warning: insight detected but LINEAR_DOCUMENT_ID not configured, skipping")
		stats.Skipped++
		if !p.cfg.DryRun {
			if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}

	slackLink, err := p.slack.GetPermalink(channelID, msg.Timestamp)
	if err != nil {
		log.Printf("  Warning: failed to get Slack permalink: %v", err)
		slackLink = ""
	}

	today := time.Now().Format("2006-01-02")
	entry := fmt.Sprintf("### %s\n%s", result.Insight.Title, result.Insight.Summary)
	if slackLink != "" {
		entry += fmt.Sprintf("\n\n[Slack thread](%s)", slackLink)
	}
	entry += fmt.Sprintf("\n\n*Added: %s*", today)

	if p.cfg.DryRun {
		log.Printf("  [DRY RUN] Would append insight to document %s:", p.cfg.LinearDocumentID)
		log.Printf("  [DRY RUN] %s", entry)
		stats.Insights++
		return nil
	}

	log.Printf("  Appending insight to Linear document %s: %s", p.cfg.LinearDocumentID, result.Insight.Title)
	if err := p.linear.AppendToDocument(p.cfg.LinearDocumentID, entry); err != nil {
		return fmt.Errorf("appending to document: %w", err)
	}

	log.Printf("  Insight added to document successfully")
	stats.Insights++

	if err := p.slack.AddSeenReaction(channelID, msg.Timestamp); err != nil {
		log.Printf("  Warning: failed to add reaction: %v", err)
	}

	return nil
}

func collectThreadImages(thread *mslack.Thread) []mslack.FileAttachment {
	var images []mslack.FileAttachment
	images = append(images, thread.Parent.Files...)
	for _, r := range thread.Replies {
		images = append(images, r.Files...)
	}
	return images
}

func (p *Processor) uploadImagesToLinear(images []mslack.FileAttachment) string {
	var parts []string
	for _, img := range images {
		log.Printf("  Downloading image %s from Slack...", img.Name)
		data, err := p.slack.DownloadFile(img.URL)
		if err != nil {
			log.Printf("  Warning: failed to download %s: %v", img.Name, err)
			continue
		}

		log.Printf("  Uploading image %s to Linear...", img.Name)
		assetURL, err := p.linear.UploadFile(img.Name, img.MimeType, data)
		if err != nil {
			log.Printf("  Warning: failed to upload %s to Linear: %v", img.Name, err)
			continue
		}

		parts = append(parts, fmt.Sprintf("![%s](%s)", img.Name, assetURL))
	}
	return strings.Join(parts, "\n")
}
