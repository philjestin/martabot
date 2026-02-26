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
	Total   int
	Seen    int
	Created int
	Skipped int
	Errors  int
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
	log.Println("Fetching messages from Slack channel...")

	messages, err := p.slack.FetchThreadParents()
	if err != nil {
		return fmt.Errorf("fetching messages: %w", err)
	}

	stats := Stats{Total: len(messages)}
	log.Printf("Found %d top-level messages", stats.Total)

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

		if err := p.processThread(ctx, msg, &stats); err != nil {
			log.Printf("  Error processing thread ts=%s: %v", msg.Timestamp, err)
			stats.Errors++
			continue
		}

		// Simple rate limiter: ~1 req/sec for Slack
		time.Sleep(1 * time.Second)
	}

	log.Printf("Done. Total=%d Seen=%d Created=%d Skipped=%d Errors=%d",
		stats.Total, stats.Seen, stats.Created, stats.Skipped, stats.Errors)

	return nil
}

func (p *Processor) processThread(ctx context.Context, msg slack.Message, stats *Stats) error {
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
		replies, err := p.slack.FetchThreadReplies(msg.Timestamp)
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

	log.Printf("  Claude decision: create_ticket=%v reason=%q", result.ShouldCreateTicket, result.Reason)

	if !result.ShouldCreateTicket {
		stats.Skipped++
		if !p.cfg.DryRun {
			if err := p.slack.AddSeenReaction(msg.Timestamp); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}

	// Get Slack thread permalink to include in the ticket
	slackLink, err := p.slack.GetPermalink(msg.Timestamp)
	if err != nil {
		log.Printf("  Warning: failed to get Slack permalink: %v", err)
		slackLink = ""
	}

	description := result.Ticket.Description
	if slackLink != "" {
		description = fmt.Sprintf("%s\n\n---\n[Slack thread](%s)", description, slackLink)
	}

	// Collect images from the thread
	images := collectThreadImages(thread)

	if p.cfg.DryRun {
		log.Printf("  [DRY RUN] Would create ticket: %s", result.Ticket.Title)
		log.Printf("  [DRY RUN] Description: %s", description)
		if len(images) > 0 {
			log.Printf("  [DRY RUN] Would upload %d image(s)", len(images))
		}
		stats.Created++
		return nil
	}

	// Upload images to Linear and append to description
	if len(images) > 0 {
		imageMarkdown := p.uploadImagesToLinear(images)
		if imageMarkdown != "" {
			description = fmt.Sprintf("%s\n\n### Attachments\n%s", description, imageMarkdown)
		}
	}

	log.Printf("  Creating Linear ticket: %s", result.Ticket.Title)
	issueResp, err := p.linear.CreateIssue(linear.IssueCreateInput{
		Title:       result.Ticket.Title,
		Description: description,
		TeamID:      p.cfg.LinearTeamID,
		ProjectID:   p.cfg.LinearProjectID,
	})
	if err != nil {
		return fmt.Errorf("creating linear issue: %w", err)
	}

	issue := issueResp.Data.IssueCreate.Issue
	log.Printf("  Created ticket %s: %s", issue.Identifier, issue.URL)

	if err := p.slack.AddSeenReaction(msg.Timestamp); err != nil {
		log.Printf("  Warning: failed to add reaction: %v", err)
	}

	if p.cfg.PostReply {
		replyText := fmt.Sprintf("Created ticket %s: %s", issue.Identifier, issue.URL)
		if err := p.slack.PostThreadReply(msg.Timestamp, replyText); err != nil {
			log.Printf("  Warning: failed to post thread reply: %v", err)
		}
	}

	stats.Created++
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
