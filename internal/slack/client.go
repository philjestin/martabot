package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

type Client struct {
	api       *slack.Client
	channelID string
	seenEmoji string
	botUserID string
}

func NewClient(token, channelID, seenEmoji string) (*Client, error) {
	api := slack.New(token)

	resp, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack auth test failed: %w", err)
	}

	return &Client{
		api:       api,
		channelID: channelID,
		seenEmoji: seenEmoji,
		botUserID: resp.UserID,
	}, nil
}

func (c *Client) FetchThreadParents() ([]slack.Message, error) {
	var all []slack.Message
	cursor := ""

	for {
		params := &slack.GetConversationHistoryParameters{
			ChannelID: c.channelID,
			Limit:     200,
			Cursor:    cursor,
		}
		resp, err := c.api.GetConversationHistory(params)
		if err != nil {
			return nil, fmt.Errorf("fetching conversation history: %w", err)
		}

		all = append(all, resp.Messages...)

		if resp.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}

	return all, nil
}

func (c *Client) HasSeenReaction(msg slack.Message) bool {
	for _, r := range msg.Reactions {
		if r.Name == c.seenEmoji {
			return true
		}
	}
	return false
}

func (c *Client) IsBotMessage(msg slack.Message) bool {
	return msg.User == c.botUserID
}

func (c *Client) FetchThreadReplies(parentTS string) ([]slack.Message, error) {
	msgs, _, _, err := c.api.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: c.channelID,
		Timestamp: parentTS,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching thread replies: %w", err)
	}

	// The first message in replies is the parent; skip it
	if len(msgs) > 1 {
		return msgs[1:], nil
	}
	return nil, nil
}

func (c *Client) BuildThread(parent slack.Message) (*Thread, error) {
	thread := &Thread{
		Parent: Message{
			User:      parent.User,
			Text:      parent.Text,
			Timestamp: parent.Timestamp,
		},
	}

	if parent.ReplyCount > 0 {
		replies, err := c.FetchThreadReplies(parent.Timestamp)
		if err != nil {
			return nil, err
		}
		for _, r := range replies {
			if r.User == c.botUserID {
				continue
			}
			thread.Replies = append(thread.Replies, Message{
				User:      r.User,
				Text:      r.Text,
				Timestamp: r.Timestamp,
			})
		}
	}

	return thread, nil
}

func (c *Client) AddSeenReaction(messageTS string) error {
	return c.api.AddReaction(c.seenEmoji, slack.ItemRef{
		Channel:   c.channelID,
		Timestamp: messageTS,
	})
}

func (c *Client) PostThreadReply(parentTS, text string) error {
	_, _, err := c.api.PostMessage(
		c.channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(parentTS),
	)
	return err
}
