package gmailsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

const GmailSendScope = "https://www.googleapis.com/auth/gmail.send"

// Reply has caller-validated content and targets a single parent in a Gmail thread.
type Reply struct {
	ThreadID        string
	Sender          string
	Recipient       string
	Subject         string
	ParentMessageID string
	MessageID       string
	Body            string
}

// ThreadMessage holds only metadata needed to check recipients, thread freshness and
// whether a previous send reached Gmail.
type ThreadMessage struct {
	ID        string
	MessageID string
	From      string
	ReplyTo   string
	Subject   string
	Sent      bool
	Automated bool
}

type ReplyClient struct {
	client *http.Client
}

func NewReplyClient(client *http.Client) *ReplyClient {
	return &ReplyClient{client: client}
}

var rfcMessageID = regexp.MustCompile(`^<[^<>@[:space:][:cntrl:]]+@[^<>@[:space:][:cntrl:]]+>$`)

func (c *ReplyClient) Send(ctx context.Context, reply Reply) (string, error) {
	raw, err := encodeReply(reply)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{
		"threadId": reply.ThreadID,
		"raw":      base64.RawURLEncoding.EncodeToString(raw),
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailBaseURL+"/messages/send", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var sent struct{ ID, ThreadID string }
	if err := c.read(req, &sent); err != nil {
		return "", err
	}
	if sent.ID == "" || sent.ThreadID != reply.ThreadID {
		return "", errors.New("gmail: reply delivery could not be confirmed")
	}
	return sent.ID, nil
}

func encodeReply(reply Reply) ([]byte, error) {
	from, fromErr := mail.ParseAddress(reply.Sender)
	to, toErr := mail.ParseAddress(reply.Recipient)
	subject := strings.TrimSpace(reply.Subject)
	if fromErr != nil || toErr != nil || from.Address != reply.Sender || to.Address != reply.Recipient ||
		reply.ThreadID == "" || !rfcMessageID.MatchString(reply.ParentMessageID) ||
		!rfcMessageID.MatchString(reply.MessageID) || len(reply.Subject) > 600 ||
		strings.ContainsFunc(reply.Subject, unicode.IsControl) || subject == "" || strings.TrimSpace(reply.Body) == "" {
		return nil, errors.New("gmail: invalid reply metadata")
	}
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	body := strings.ReplaceAll(strings.ReplaceAll(reply.Body, "\r\n", "\n"), "\n", "\r\n")
	headers := []string{
		"From: " + from.String(),
		"To: " + to.String(),
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject),
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID: " + reply.MessageID,
		"In-Reply-To: " + reply.ParentMessageID,
		"References: " + reply.ParentMessageID,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n"), nil
}

// Thread returns the thread in Gmail's chronological order, with no message bodies.
func (c *ReplyClient) Thread(ctx context.Context, threadID string) ([]ThreadMessage, error) {
	query := url.Values{"format": {"metadata"}, "metadataHeaders": {
		"Message-ID", "From", "Reply-To", "Subject", "Auto-Submitted", "List-Id", "Precedence",
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		gmailBaseURL+"/threads/"+url.PathEscape(threadID)+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var thread struct {
		ID       string
		Messages []struct {
			ID       string
			LabelIDs []string
			Payload  struct {
				Headers []struct{ Name, Value string }
			}
		}
	}
	if err := c.read(req, &thread); err != nil {
		return nil, err
	}
	if thread.ID != threadID || len(thread.Messages) == 0 {
		return nil, errors.New("gmail: unexpected thread")
	}
	messages := make([]ThreadMessage, 0, len(thread.Messages))
	for _, item := range thread.Messages {
		if slices.Contains(item.LabelIDs, "DRAFT") || slices.Contains(item.LabelIDs, "TRASH") {
			continue
		}
		out := ThreadMessage{
			ID: item.ID, Sent: slices.Contains(item.LabelIDs, "SENT"),
			Automated: slices.Contains(item.LabelIDs, "SPAM"),
		}
		for _, header := range item.Payload.Headers {
			switch strings.ToLower(header.Name) {
			case "message-id":
				out.MessageID = header.Value
			case "from":
				out.From = header.Value
			case "reply-to":
				out.ReplyTo = header.Value
			case "subject":
				out.Subject = header.Value
			case "auto-submitted":
				out.Automated = out.Automated || !strings.EqualFold(header.Value, "no")
			case "list-id":
				out.Automated = true
			case "precedence":
				value := strings.ToLower(header.Value)
				out.Automated = out.Automated || value == "bulk" || value == "list" || value == "junk"
			}
		}
		messages = append(messages, out)
	}
	return messages, nil
}

func (c *ReplyClient) read(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("gmail reply: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NewAPIError("gmail reply", resp)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
