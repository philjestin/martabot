package slack

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/slack-go/slack"
)

type Client struct {
	api         *slack.Client
	token       string
	seenEmoji   string
	ignoreEmoji string
	botUserID   string

	userNamesMu sync.Mutex
	userNames   map[string]string
}

func NewClient(token, seenEmoji, ignoreEmoji string) (*Client, error) {
	api := slack.New(token)

	resp, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack auth test failed: %w", err)
	}

	return &Client{
		api:         api,
		token:       token,
		seenEmoji:   seenEmoji,
		ignoreEmoji: ignoreEmoji,
		botUserID:   resp.UserID,
		userNames:   make(map[string]string),
	}, nil
}

func (c *Client) FetchThreadParents(channelID string) ([]slack.Message, error) {
	var all []slack.Message
	cursor := ""

	for {
		params := &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
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

func (c *Client) HasIgnoreReaction(msg slack.Message) bool {
	for _, r := range msg.Reactions {
		if r.Name == c.ignoreEmoji {
			return true
		}
	}
	return false
}

func (c *Client) IsBotMessage(msg slack.Message) bool {
	return msg.User == c.botUserID
}

func (c *Client) FetchThreadReplies(channelID, parentTS string) ([]slack.Message, error) {
	msgs, _, _, err := c.api.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channelID,
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

func (c *Client) BuildThread(channelID string, parent slack.Message) (*Thread, error) {
	thread := &Thread{
		Parent: Message{
			User:      parent.User,
			Text:      parent.Text,
			Timestamp: parent.Timestamp,
			Files:     ExtractImageFiles(parent.Files),
		},
	}

	if parent.ReplyCount > 0 {
		replies, err := c.FetchThreadReplies(channelID, parent.Timestamp)
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
				Files:     ExtractImageFiles(r.Files),
			})
		}
	}

	return thread, nil
}

func (c *Client) AddSeenReaction(channelID, messageTS string) error {
	return c.api.AddReaction(c.seenEmoji, slack.ItemRef{
		Channel:   channelID,
		Timestamp: messageTS,
	})
}

func (c *Client) GetPermalink(channelID, messageTS string) (string, error) {
	permalink, err := c.api.GetPermalink(&slack.PermalinkParameters{
		Channel: channelID,
		Ts:      messageTS,
	})
	if err != nil {
		return "", fmt.Errorf("getting permalink: %w", err)
	}
	return permalink, nil
}

func (c *Client) PostThreadReply(channelID, parentTS, text string) error {
	_, _, err := c.api.PostMessage(
		channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(parentTS),
	)
	return err
}

func (c *Client) ResolveUserName(userID string) string {
	c.userNamesMu.Lock()
	if name, ok := c.userNames[userID]; ok {
		c.userNamesMu.Unlock()
		return name
	}
	c.userNamesMu.Unlock()

	user, err := c.api.GetUserInfo(userID)
	if err != nil {
		c.userNamesMu.Lock()
		c.userNames[userID] = userID
		c.userNamesMu.Unlock()
		return userID
	}

	name := user.Profile.DisplayName
	if name == "" {
		name = user.RealName
	}
	if name == "" {
		name = userID
	}

	c.userNamesMu.Lock()
	c.userNames[userID] = name
	c.userNamesMu.Unlock()
	return name
}

func ExtractImageFiles(files []slack.File) []FileAttachment {
	var images []FileAttachment
	for _, f := range files {
		if !strings.HasPrefix(f.Mimetype, "image/") {
			continue
		}
		url := f.URLPrivateDownload
		if url == "" {
			url = f.URLPrivate
		}
		if url == "" {
			continue
		}
		images = append(images, FileAttachment{
			Name:     f.Name,
			MimeType: f.Mimetype,
			URL:      url,
		})
	}
	return images
}

func (c *Client) DownloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading download body: %w", err)
	}
	return data, nil
}
