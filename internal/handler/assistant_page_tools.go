package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/assistant"
)

// pageSnapshot is what the extension reports about the tab the candidate is
// looking at. The extension already trims the text to a few thousand characters,
// so the whole snapshot is small enough to enter the conversation as-is.
type pageSnapshot struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Headline string `json:"headline"`
	Text     string `json:"text"`
}

// noBrowserAttached is what the model is told when the call reaches no browser —
// the ordinary case, not a fault. It names the remedy because that sentence is
// the model's only way to get the user to fix it.
const noBrowserAttached = "no browser is attached: the freehire side panel has to be open on the page you mean, in this browser"

// readCurrentPageTool lets the agent see the page the caller's browser is
// showing. It attaches to that user's browser-tool channel as an in-process
// harness for the length of one call, exactly as the agentic autofill does — the
// channel is keyed by the id the authenticating middleware resolved, so the tool
// cannot reach another user's browser even if the model asks it to.
func (h *assistantHandlers) readCurrentPageTool() assistant.Tool {
	return assistant.Tool{
		Name: "read_current_page",
		Description: "Read the page the user is currently looking at in their browser: its url, " +
			"title, headline and visible text. Call it whenever the user refers to what is in " +
			"front of them (\"this role\", \"this company\", \"here\"), and again after they may " +
			"have navigated — it reports what the tab shows now. It works on any site, not only " +
			"on vacancies freehire already has.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			if h.browserTools == nil {
				return nil, errors.New(noBrowserAttached)
			}
			caller := h.browserTools.NewCaller(userID)
			defer caller.Close()

			raw, err := caller.Call(ctx, "read_page", nil)
			if err != nil {
				// Every failure here is the same thing from the model's side: it cannot
				// see the page. The relay's own wording ("the browser extension is not
				// connected") is kept for the diagnosis and the remedy appended to it.
				return nil, fmt.Errorf("%v — %s", err, noBrowserAttached)
			}

			var snap pageSnapshot
			if err := json.Unmarshal(raw, &snap); err != nil {
				return nil, fmt.Errorf("the browser answered with something that is not a page: %w", err)
			}
			return snap, nil
		},
	}
}
