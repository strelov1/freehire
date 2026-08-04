package gmailsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ical"
)

const gmailBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"

// apiReader is the live GmailReader over the Gmail REST API, using a token-bearing
// HTTP client (minted from the user's refresh token). learned holds the promoted
// self-learning domains to union into the search query.
type apiReader struct {
	client  *http.Client
	learned []string
}

// NewAPIReader wraps a token-bearing HTTP client as a GmailReader, unioning the
// learned domains into its search query.
func NewAPIReader(client *http.Client, learned []string) GmailReader {
	return &apiReader{client: client, learned: learned}
}

// ListATSMessageIDs pages through the ATS-scoped search, returning all matching ids.
func (r *apiReader) ListATSMessageIDs(ctx context.Context, selfAddr string, afterUnix int64) ([]string, error) {
	q := BuildQueryFor(selfAddr, afterUnix, r.learned)
	var ids []string
	pageToken := ""
	for {
		u := fmt.Sprintf("%s/messages?maxResults=100&q=%s", gmailBaseURL, url.QueryEscape(q))
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var page struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := r.getJSON(ctx, u, &page); err != nil {
			return nil, err
		}
		for _, m := range page.Messages {
			ids = append(ids, m.ID)
		}
		if page.NextPageToken == "" {
			return ids, nil
		}
		pageToken = page.NextPageToken
	}
}

// ListThreadMessageIDs returns the ids of every message in a thread.
func (r *apiReader) ListThreadMessageIDs(ctx context.Context, threadID string) ([]string, error) {
	u := fmt.Sprintf("%s/threads/%s?format=minimal", gmailBaseURL, url.PathEscape(threadID))
	var t struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := r.getJSON(ctx, u, &t); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(t.Messages))
	for _, m := range t.Messages {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// GetMessage fetches one full message and parses it.
func (r *apiReader) GetMessage(ctx context.Context, id string) (Message, error) {
	u := fmt.Sprintf("%s/messages/%s?format=full", gmailBaseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Message{}, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("gmail: get message %s: %s", id, resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, err
	}
	return parseMessage(raw)
}

func (r *apiReader) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gmail: %s: %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// --- pure parsing -----------------------------------------------------------

type gmailPart struct {
	MimeType string        `json:"mimeType"`
	Headers  []gmailHeader `json:"headers"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailMessage struct {
	ID           string    `json:"id"`
	ThreadID     string    `json:"threadId"`
	InternalDate string    `json:"internalDate"`
	Payload      gmailPart `json:"payload"`
}

// parseMessage turns a Gmail `format=full` message into a domain Message: headers,
// decoded text/HTML bodies, and receipt time from internalDate.
func parseMessage(raw []byte) (Message, error) {
	var gm gmailMessage
	if err := json.Unmarshal(raw, &gm); err != nil {
		return Message{}, fmt.Errorf("gmail: decode message: %w", err)
	}
	m := Message{ID: gm.ID, ThreadID: gm.ThreadID}

	if ms, err := strconv.ParseInt(gm.InternalDate, 10, 64); err == nil {
		m.ReceivedAt = time.UnixMilli(ms)
	}

	from := headerValue(gm.Payload.Headers, "From")
	if addr, err := mail.ParseAddress(from); err == nil {
		m.FromName, m.FromAddr = addr.Name, addr.Address
	} else {
		m.FromAddr = from
	}
	m.Subject = headerValue(gm.Payload.Headers, "Subject")

	m.BodyText, m.BodyHTML = bodies(gm.Payload)
	m.CalendarUID = calendarUID(gm.Payload)
	return m, nil
}

func headerValue(headers []gmailHeader, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// bodies walks the MIME tree, returning the first text/plain and text/html bodies.
func bodies(p gmailPart) (text, html string) {
	var walk func(part gmailPart)
	walk = func(part gmailPart) {
		switch part.MimeType {
		case "text/plain":
			if text == "" {
				text = decodeB64URL(part.Body.Data)
			}
		case "text/html":
			if html == "" {
				html = decodeB64URL(part.Body.Data)
			}
		}
		for _, sub := range part.Parts {
			walk(sub)
		}
	}
	walk(p)
	return text, html
}

// calendarUID walks the MIME tree for a text/calendar part and reads the meeting's
// identifier out of it. A part that carries none, or that does not parse, yields "" —
// an invitation whose meeting we cannot identify is still an invitation, and the mail
// must not be lost over it.
func calendarUID(p gmailPart) string {
	var found string
	var walk func(part gmailPart)
	walk = func(part gmailPart) {
		if found != "" {
			return
		}
		if strings.EqualFold(part.MimeType, "text/calendar") {
			found = ical.UID([]byte(decodeB64URL(part.Body.Data)))
		}
		for _, sub := range part.Parts {
			walk(sub)
		}
	}
	walk(p)
	return found
}

// decodeB64URL decodes Gmail's URL-safe base64 body, tolerating missing padding.
func decodeB64URL(s string) string {
	if s == "" {
		return ""
	}
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// Search runs an arbitrary query and returns the matching messages in full.
//
// One page only. The recall query is already scoped to one employer inside one window, so a
// result that overflows a page is a query that failed to narrow rather than a mailbox that
// needs paging — and the caller is a person waiting at a button.
func (r *apiReader) Search(ctx context.Context, query string, limit int) ([]Message, error) {
	if query == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s/messages?maxResults=%d&q=%s", gmailBaseURL, limit, url.QueryEscape(query))
	var page struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := r.getJSON(ctx, u, &page); err != nil {
		return nil, err
	}

	out := make([]Message, 0, len(page.Messages))
	for _, m := range page.Messages {
		msg, err := r.GetMessage(ctx, m.ID)
		if err != nil {
			// One unreadable message must not lose the rest: the caller is choosing from
			// what we found, and a short list beats an error nobody can act on.
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}
