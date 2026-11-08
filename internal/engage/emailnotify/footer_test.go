package emailnotify

import (
	"context"
	"errors"
	"html"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// An optional mail must tell the reader who wrote and why in both parts, and a bare
// From gets the product's name; an essential mail is held to neither footer.
func TestClient_FooterAndSenderName(t *testing.T) {
	const url = "https://freehire.me/unsubscribe?t=a&g=alerts"
	good := Message{
		From: "n@mail.freehire.me", To: "t", Subject: "s",
		HTML: `<a href="` + html.EscapeString(url) + `">Unsubscribe</a>`, Text: "x" + emailprefs.TextFooter(url),
		Group: emailprefs.GroupAlerts, UnsubscribeURL: url,
	}
	noText, noHTML := good, good
	noText.Text = "x\nUnsubscribe: " + url
	noHTML.HTML = "<p>x</p>"
	for name, m := range map[string]Message{"text without footer": noText, "html without link": noHTML} {
		if err := (&Client{ses: &fakeSES{}}).Send(context.Background(), m); !errors.Is(err, ErrNoWayOut) {
			t.Errorf("%s: got %v, want ErrNoWayOut", name, err)
		}
	}

	fake := &fakeSES{}
	if err := (&Client{ses: fake}).Send(context.Background(), good); err != nil {
		t.Fatalf("good mail refused: %v", err)
	}
	if got := deref(fake.in.FromEmailAddress); got != `"freehire" <n@mail.freehire.me>` {
		t.Errorf("from = %q, want the bare address named freehire", got)
	}
	if !strings.Contains(emailprefs.TextFooter(url), "This email is from freehire") {
		t.Error("TextFooter no longer says who is writing")
	}

	named := good
	named.From = "Ilya from freehire <n@mail.freehire.me>"
	fake = &fakeSES{}
	if err := (&Client{ses: fake}).Send(context.Background(), named); err != nil {
		t.Fatal(err)
	}
	if got := deref(fake.in.FromEmailAddress); got != named.From {
		t.Errorf("a named sender was renamed: %q", got)
	}

	essential := Message{From: "n@mail.freehire.me", To: "t", Subject: "s", HTML: "<p>code</p>", Text: "code", Group: emailprefs.GroupEssential}
	if err := (&Client{ses: &fakeSES{}}).Send(context.Background(), essential); err != nil {
		t.Errorf("essential mail without footer refused: %v", err)
	}
}
