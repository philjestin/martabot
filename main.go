package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/pmiddleton/martabot/internal/bot"
	"github.com/pmiddleton/martabot/internal/claude"
	"github.com/pmiddleton/martabot/internal/config"
	"github.com/pmiddleton/martabot/internal/linear"
	mslack "github.com/pmiddleton/martabot/internal/slack"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Evaluate threads but don't create tickets or react")
	verbose := flag.Bool("verbose", false, "Enable debug logging")
	flag.Parse()

	// Load .env file if present (not an error if missing)
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Warning: error loading .env file: %v", err)
		}
	}

	cfg, err := config.Load(*dryRun, *verbose)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	if cfg.DryRun {
		log.Println("Running in dry-run mode (no tickets will be created, no reactions added)")
	}

	sc, err := mslack.NewClient(cfg.SlackBotToken, cfg.SeenEmoji, cfg.IgnoreEmoji)
	if err != nil {
		log.Fatalf("Failed to initialize Slack client: %v", err)
	}

	cc, err := claude.NewClient(context.Background(), cfg.GCPRegion, cfg.GCPProject, cfg.ClaudeModel)
	if err != nil {
		log.Fatalf("Failed to initialize Claude client: %v", err)
	}
	lc := linear.NewClient(cfg.LinearAPIKey)

	p := bot.NewProcessor(cfg, sc, cc, lc)

	if err := p.Run(context.Background()); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
