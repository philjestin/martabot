# Martabot

A one-shot CLI tool that monitors a Slack feedback channel, uses Claude (via Vertex AI) to evaluate whether threads contain enough detail for actionable tickets, and creates Linear tickets when they do.

## How it works

1. Fetches all top-level messages from a configured Slack channel
2. Skips any message already marked with the "seen" emoji (`:eyes:`)
3. For unseen threads, fetches all replies
4. Sends the formatted thread to Claude — it decides if a ticket is warranted and generates title, description, and priority
5. If yes: creates a Linear ticket via GraphQL API
6. Adds the "seen" emoji reaction to the Slack message
7. Optionally replies in the thread with a link to the created ticket

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
| `SLACK_CHANNEL_ID` | yes | — | Channel ID to monitor |
| `SEEN_EMOJI` | no | `eyes` | Emoji name for marking processed threads |
| `POST_REPLY` | no | `true` | Reply in thread with ticket link |
| `GCP_PROJECT` | yes | — | Google Cloud project ID |
| `GCP_REGION` | yes | — | Vertex AI region (e.g. `us-east5`) |
| `CLAUDE_MODEL` | no | `claude-sonnet-4-5` | Model for evaluation |
| `LINEAR_API_KEY` | yes | — | Linear personal API key |
| `LINEAR_TEAM_ID` | yes | — | Linear team UUID for ticket creation |

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
3. Click **Install to Workspace** and authorize
4. Copy the **Bot User OAuth Token** (`xoxb-...`) into your `.env`
5. Invite the bot to your channel: `/invite @YourBotName`

## Linear API key

Go to https://linear.app/settings/api and create a personal API key.

The `LINEAR_TEAM_ID` is the UUID of the team where tickets should be created. You can find it via the Linear API or in the URL when viewing a team's settings.
