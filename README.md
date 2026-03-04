# Martabot

A one-shot CLI tool that monitors Slack channels, uses Claude (via Vertex AI) to classify threads, and either creates Linear tickets for product feedback or appends summaries to a Linear document for insights/demos/learnings.

## How it works

1. Fetches all top-level messages from configured Slack channels (a feedback channel and an optional insights channel)
2. Skips bot messages, threads with the "seen" emoji (`:eyes:`), and threads with the "ignore" emoji (`:no_entry_sign:`)
3. For unseen threads, fetches all replies and resolves Slack user IDs to display names
4. Sends the formatted thread to Claude, which classifies it into one of three categories:
   - **Feedback** (bug reports, feature requests, UX issues) — creates Linear tickets
   - **Insight** (demos, learnings, customer insights) — appends a summary to a Linear document
   - **Skip** (off-topic, logistics, chatter) — marks as seen and moves on
5. For feedback: downloads any screenshots/images from the thread, uploads them to Linear, and creates tickets via the GraphQL API with the description, Slack thread link, and images
6. For insights: formats a markdown entry (title, summary, Slack link, date) and appends it to a configured Linear document
7. Adds the "seen" emoji reaction to the Slack message
8. Optionally replies in the thread with a link to created tickets

## Prerequisites

- Go 1.21+
- Google Cloud access to Claude via Vertex AI (`gcloud auth application-default login`)
- A Slack App with a bot token ([setup guide](#slack-app-setup))
- A Linear API key

## Setup

```bash
cp .env.example .env
# Fill in your values in .env
```

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SLACK_BOT_TOKEN` | yes | — | Slack bot token (`xoxb-...`) |
| `SLACK_CHANNEL_ID` | yes | — | Feedback channel ID to monitor |
| `INSIGHTS_CHANNEL_ID` | no | — | Second channel for insights/demos/learnings |
| `SEEN_EMOJI` | no | `eyes` | Emoji name for marking processed threads |
| `IGNORE_EMOJI` | no | `no_entry_sign` | Emoji name for manually ignoring threads |
| `POST_REPLY` | no | `true` | Reply in thread with ticket link |
| `GCP_PROJECT` | yes | — | Google Cloud project ID |
| `GCP_REGION` | yes | — | Vertex AI region (e.g. `us-east5`) |
| `CLAUDE_MODEL` | no | `claude-sonnet-4-5` | Model for evaluation |
| `LINEAR_API_KEY` | yes | — | Linear personal API key |
| `LINEAR_TEAM_ID` | yes | — | Linear team UUID for ticket creation |
| `LINEAR_PROJECT_ID` | no | — | Linear project UUID (assigns tickets to a project) |
| `LINEAR_DOCUMENT_ID` | no | — | Linear document UUID for appending insights |

## Usage

```bash
# Dry run — evaluate threads but don't create tickets or react
go run main.go --dry-run --verbose

# Run for real
go run main.go
```

### Flags

- `--dry-run` — Evaluate threads with Claude but don't create Linear tickets or add Slack reactions
- `--verbose` — Print debug output including thread text and Claude responses

## Slack app setup

1. Go to https://api.slack.com/apps → **Create New App** → **From scratch**
2. Under **OAuth & Permissions**, add these **Bot Token Scopes**:
   - `channels:history`
   - `channels:read`
   - `reactions:read`
   - `reactions:write`
   - `chat:write`
   - `users:read` (for resolving user display names)
   - `files:read` (for downloading images/screenshots)
3. Click **Install to Workspace** and authorize
4. Copy the **Bot User OAuth Token** (`xoxb-...`) into your `.env`
5. Invite the bot to your channel: `/invite @YourBotName`

## Linear API key

Go to https://linear.app/settings/api and create a personal API key.

The `LINEAR_TEAM_ID` is the UUID of the team where tickets should be created. You can find it via the Linear API or in the URL when viewing a team's settings.
