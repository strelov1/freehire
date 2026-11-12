package handler

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/billing"
)

func TestSupportRegistryHoldsExactlyThreeTools(t *testing.T) {
	got := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetSupport}, uuid.New()).Names()
	want := []string{"escalate_to_human", "my_account", "report_problem"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("support tools = %v, want %v", got, want)
	}
}

func TestSupportPromptNamesItsToolsAndNoneOfTheInbox(t *testing.T) {
	p := assistant.SystemPrompt(assistant.PresetSupport, "en")
	for _, want := range []string{"my_account", "escalate_to_human"} {
		if !strings.Contains(p, want) {
			t.Errorf("the support prompt never names %q", want)
		}
	}
	for _, banned := range []string{"mail_search", "mail_get", "inbox"} {
		if strings.Contains(strings.ToLower(p), banned) {
			t.Errorf("the support prompt mentions %q, which that preset does not have", banned)
		}
	}
}

func TestCleanSupportTextIsInert(t *testing.T) {
	got := cleanSupportText("hello\r\nBcc: evil@example.test\x00\x1b[31m\tworld", 1200)
	for _, bad := range []string{"\r", "\n", "\t", "\x00", "\x1b"} {
		if strings.Contains(got, bad) {
			t.Errorf("cleaned text still holds %q: %q", bad, got)
		}
	}
	if got != "hello Bcc: evil@example.test [31m world" {
		t.Errorf("cleaned text = %q", got)
	}
	if n := len([]rune(cleanSupportText(strings.Repeat("é", 5000), 1200))); n != 1200 {
		t.Errorf("length cap = %d runes, want 1200", n)
	}
}

func TestSafeReplyTo(t *testing.T) {
	for _, tc := range []struct {
		email    string
		verified bool
		want     string
	}{
		{"a@example.test", true, "a@example.test"},
		{"a@example.test", false, ""},
		{"", true, ""},
		{"a@example.test\r\nBcc: x@example.test", true, ""},
		{"Name <a@example.test>", true, ""},
		{"a@example.test, b@example.test", true, ""},
		{`"a b"@example.test`, true, ""},
	} {
		if got := safeReplyTo(tc.email, tc.verified); got != tc.want {
			t.Errorf("safeReplyTo(%q, %v) = %q, want %q", tc.email, tc.verified, got, tc.want)
		}
	}
}

func TestSupportTurnIsCappedAtThreeSteps(t *testing.T) {
	if got := turnBounds(assistant.Session{Preset: assistant.PresetSupport}, "").MaxSteps; got != 3 {
		t.Errorf("support MaxSteps = %d, want 3", got)
	}
}

func TestAccountPricesFormatsCentsAsAnAmount(t *testing.T) {
	got := accountPrices([]billing.PublicPrice{
		{Tier: "pro", AmountCents: 500, Currency: "usd", Interval: "month"},
		{Tier: "api", AmountCents: 2905, Currency: "eur", Interval: "year"},
	})
	want := []accountPrice{
		{Plan: "pro", Amount: "5.00 USD", Interval: "month"},
		{Plan: "api", Amount: "29.05 EUR", Interval: "year"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("accountPrices = %+v, want %+v", got, want)
	}
}
