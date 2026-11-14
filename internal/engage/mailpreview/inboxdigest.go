package mailpreview

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/engage/inboxdigest"
)

func inboxDigestSample(baseURL string) (Sample, error) {
	return sample("inbox-digest", "Inbox / Daily hiring mail", func(c *capture) error {
		at := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
		message, err := inboxdigest.NewMailer("alerts@freehire.me", baseURL, previewSecret).Render(
			inboxdigest.Candidate{UserID: previewUserID, Email: "someone@example.com", Timezone: "Europe/London"},
			inboxdigest.Snapshot{Total: 4, Items: []inboxdigest.Item{
				{ID: 1, Subject: "Backend engineering opportunity", Sender: "Acme recruiting", Signal: "screening", Snippet: "Would you be interested in discussing our backend engineering role?", ReceivedAt: at},
				{ID: 2, Subject: "Interview availability", Sender: "Example hiring team", Signal: "interview_invitation", Snippet: "Please share your availability for a technical interview next week.", ReceivedAt: at},
				{ID: 3, Subject: "Thanks for your application", Sender: "Sample company", Signal: "acknowledgement", Replied: true, ReceivedAt: at},
			}},
		)
		if err != nil {
			return err
		}
		return c.Send(context.Background(), message)
	})
}
