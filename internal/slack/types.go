package slack

import (
	"fmt"
	"strings"
)

type FileAttachment struct {
	Name     string
	MimeType string
	URL      string
}

type Message struct {
	User      string
	Text      string
	Timestamp string
	Files     []FileAttachment
}

type Thread struct {
	Parent  Message
	Replies []Message
}

func (t *Thread) FormatForClaude(maxChars int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "[Original message by %s]\n%s\n", t.Parent.User, t.Parent.Text)

	for _, r := range t.Replies {
		fmt.Fprintf(&b, "\n[Reply by %s]\n%s\n", r.User, r.Text)
	}

	s := b.String()
	if maxChars > 0 && len(s) > maxChars {
		s = s[:maxChars] + "\n...(truncated)"
	}
	return s
}
