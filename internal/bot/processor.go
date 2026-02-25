package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pmiddleton/martabot/internal/claude"
	"github.com/pmiddleton/martabot/internal/config"
	"github.com/pmiddleton/martabot/internal/linear"
	mslack "github.com/pmiddleton/martabot/internal/slack"
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

		if err := p.processThread(ctx, msg.Timestamp, msg.User, msg.Text, msg.ReplyCount, &stats); err != nil {
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

func (p *Processor) processThread(ctx context.Context, parentTS, user, text string, replyCount int, stats *Stats) error {
	log.Printf("  Processing thread ts=%s (user=%s, replies=%d)", parentTS, user, replyCount)

	thread := &mslack.Thread{
		Parent: mslack.Message{
			User:      user,
			Text:      text,
			Timestamp: parentTS,
		},
	}

	if replyCount > 0 {
		replies, err := p.slack.FetchThreadReplies(parentTS)
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
			})
		}
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
			if err := p.slack.AddSeenReaction(parentTS); err != nil {
				log.Printf("  Warning: failed to add reaction: %v", err)
			}
		}
		return nil
	}

	if p.cfg.DryRun {
		log.Printf("  [DRY RUN] Would create ticket: %s (priority=%d)", result.Ticket.Title, result.Ticket.Priority)
		log.Printf("  [DRY RUN] Description: %s", result.Ticket.Description)
		stats.Created++
		return nil
	}

	log.Printf("  Creating Linear ticket: %s", result.Ticket.Title)
	issueResp, err := p.linear.CreateIssue(linear.IssueCreateInput{
		Title:       result.Ticket.Title,
		Description: result.Ticket.Description,
		TeamID:      p.cfg.LinearTeamID,
		Priority:    result.Ticket.Priority,
		ProjectID:   p.cfg.LinearProjectID,
	})
	if err != nil {
		return fmt.Errorf("creating linear issue: %w", err)
	}

	issue := issueResp.Data.IssueCreate.Issue
	log.Printf("  Created ticket %s: %s", issue.Identifier, issue.URL)

	if err := p.slack.AddSeenReaction(parentTS); err != nil {
		log.Printf("  Warning: failed to add reaction: %v", err)
	}

	if p.cfg.PostReply {
		replyText := fmt.Sprintf("Created ticket %s: %s", issue.Identifier, issue.URL)
		if err := p.slack.PostThreadReply(parentTS, replyText); err != nil {
			log.Printf("  Warning: failed to post thread reply: %v", err)
		}
	}

	stats.Created++
	return nil
}
