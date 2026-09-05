package promo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepo is an in-memory stand-in for the four tables. Every method records what it was
// asked, so a test can assert that a refusal happened BEFORE the database was touched —
// which is the whole point of the self-referral and normalisation checks.
type fakeRepo struct {
	// The reward ledger half of Repository, promoted so the two halves can be set up
	// independently — most tests here care about codes and never touch a reward.
	*fakeLedger

	codes       map[string]int16 // usable codes → percentage
	redeemedBy  map[int64]bool
	redeemedPct map[int64]int16
	inviteCodes map[int64]string
	owners      map[string]int64 // invite code → referrer
	attributed  map[int64]int64  // referee → referrer
	stats       map[int64]Stats
	pending     map[int64]bool

	previewCalls int
	redeemCalls  int
	attributions int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		fakeLedger:  newFakeLedger(),
		codes:       map[string]int16{},
		redeemedBy:  map[int64]bool{},
		redeemedPct: map[int64]int16{},
		inviteCodes: map[int64]string{},
		owners:      map[string]int64{},
		attributed:  map[int64]int64{},
		stats:       map[int64]Stats{},
		pending:     map[int64]bool{},
	}
}

func (f *fakeRepo) PreviewCode(_ context.Context, code string) (int16, error) {
	f.previewCalls++
	pct, ok := f.codes[code]
	if !ok {
		return 0, ErrNotUsable
	}
	return pct, nil
}

func (f *fakeRepo) Redeem(_ context.Context, userID int64, code string) (int16, error) {
	f.redeemCalls++
	pct, ok := f.codes[code]
	if !ok || f.redeemedBy[userID] {
		return 0, ErrNotUsable
	}
	f.redeemedBy[userID] = true
	f.redeemedPct[userID] = pct
	return pct, nil
}

func (f *fakeRepo) HasRedeemed(_ context.Context, userID int64) (bool, error) {
	return f.redeemedBy[userID], nil
}

func (f *fakeRepo) RedeemedPercent(_ context.Context, userID int64) (int16, error) {
	return f.redeemedPct[userID], nil
}

func (f *fakeRepo) EnsureInviteCode(_ context.Context, userID int64, code string) (string, error) {
	if held, ok := f.inviteCodes[userID]; ok {
		return held, nil
	}
	if _, taken := f.owners[code]; taken {
		return "", ErrCodeTaken
	}
	f.inviteCodes[userID] = code
	f.owners[code] = userID
	return code, nil
}

func (f *fakeRepo) ReferrerByCode(_ context.Context, code string) (int64, error) {
	owner, ok := f.owners[code]
	if !ok {
		return 0, ErrNotUsable
	}
	return owner, nil
}

func (f *fakeRepo) Attribute(_ context.Context, referrerID, refereeID int64) (bool, error) {
	f.attributions++
	if _, already := f.attributed[refereeID]; already {
		return false, nil
	}
	f.attributed[refereeID] = referrerID
	f.pending[refereeID] = true
	return true, nil
}

func (f *fakeRepo) Stats(_ context.Context, userID int64) (Stats, error) {
	return f.stats[userID], nil
}

func (f *fakeRepo) HasPendingInvite(_ context.Context, userID int64) (bool, error) {
	return f.pending[userID], nil
}

func newTestService(repo *fakeRepo) *Service {
	return New(repo, Config{SiteURL: "https://example.test"})
}

func TestPreviewFoldsTheCodeUp(t *testing.T) {
	repo := newFakeRepo()
	repo.codes["ZZTEST90"] = 90
	svc := newTestService(repo)

	pct, err := svc.Preview(context.Background(), 1, "  zztest90 ")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if pct != 90 {
		t.Fatalf("percent = %d, want 90 — a code typed in lower case must still resolve", pct)
	}
}

func TestPreviewRefusesAnUnknownCodeWithoutSayingWhy(t *testing.T) {
	svc := newTestService(newFakeRepo())

	_, err := svc.Preview(context.Background(), 1, "ZZNONE00")
	if !errors.Is(err, ErrNotUsable) {
		t.Fatalf("err = %v, want ErrNotUsable — the refusal must not distinguish "+
			"'no such code' from 'not eligible', or it is an oracle for guessing codes", err)
	}
}

func TestPreviewTellsAnAccountThatHasAlreadyRedeemed(t *testing.T) {
	repo := newFakeRepo()
	repo.codes["ZZTEST90"] = 90
	repo.redeemedBy[7] = true
	svc := newTestService(repo)

	_, err := svc.Preview(context.Background(), 7, "ZZTEST90")
	if !errors.Is(err, ErrAlreadyRedeemed) {
		t.Fatalf("err = %v, want ErrAlreadyRedeemed — this is a fact about the caller, "+
			"not about the code, so saying it leaks nothing", err)
	}
}

func TestPreviewRejectsAMalformedCodeWithoutAQuery(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	if _, err := svc.Preview(context.Background(), 1, "no spaces allowed!"); !errors.Is(err, ErrNotUsable) {
		t.Fatalf("err = %v, want ErrNotUsable", err)
	}
	if repo.previewCalls != 0 {
		t.Fatalf("previewCalls = %d, want 0 — a code the table could never hold is refused "+
			"before it reaches the rate-limited read", repo.previewCalls)
	}
}

func TestRedeemReturnsThePercentage(t *testing.T) {
	repo := newFakeRepo()
	repo.codes["ZZTEST90"] = 90
	svc := newTestService(repo)

	pct, err := svc.Redeem(context.Background(), 1, "zztest90")
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if pct != 90 {
		t.Fatalf("percent = %d, want 90", pct)
	}
}

func TestRedeemRefusesASecondCodeForOneAccount(t *testing.T) {
	repo := newFakeRepo()
	repo.codes["ZZTEST90"] = 90
	repo.codes["ZZTEST50"] = 50
	svc := newTestService(repo)

	if _, err := svc.Redeem(context.Background(), 1, "ZZTEST90"); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}
	_, err := svc.Redeem(context.Background(), 1, "ZZTEST50")
	if !errors.Is(err, ErrAlreadyRedeemed) {
		t.Fatalf("err = %v, want ErrAlreadyRedeemed — stacking two percentages is how a "+
			"subscription becomes free by accident", err)
	}
}

func TestLinkIsStableAndAbsolute(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	first, err := svc.Link(context.Background(), 42)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	second, err := svc.Link(context.Background(), 42)
	if err != nil {
		t.Fatalf("second Link: %v", err)
	}
	if first != second {
		t.Fatalf("Link returned %q then %q — an invite code is minted once and never rotates",
			first, second)
	}
	if !strings.HasPrefix(first, "https://example.test/r/") {
		t.Fatalf("Link = %q, want an absolute URL under the configured site", first)
	}
}

func TestLinkMintsAnUnguessableCode(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	seen := map[string]bool{}
	for id := int64(1); id <= 50; id++ {
		link, err := svc.Link(context.Background(), id)
		if err != nil {
			t.Fatalf("Link(%d): %v", id, err)
		}
		code := link[strings.LastIndex(link, "/")+1:]
		if len(code) < 16 {
			t.Fatalf("code %q is %d characters — an invite code appears in a public URL, so "+
				"a short one turns the invite link into an account enumerator", code, len(code))
		}
		if seen[code] {
			t.Fatalf("code %q was minted twice", code)
		}
		seen[code] = true
	}
}

func TestAttributeRecordsTheReferrer(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)
	link, err := svc.Link(context.Background(), 1)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	code := link[strings.LastIndex(link, "/")+1:]

	if err := svc.Attribute(context.Background(), 2, code); err != nil {
		t.Fatalf("Attribute: %v", err)
	}
	if repo.attributed[2] != 1 {
		t.Fatalf("referee 2 attributed to %d, want 1", repo.attributed[2])
	}
}

func TestAttributeIgnoresAnUnknownCode(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	if err := svc.Attribute(context.Background(), 2, "notacode000000000"); err != nil {
		t.Fatalf("Attribute: %v — the value came out of a cookie, and a cookie is whatever "+
			"the visitor put in it; that is not an error", err)
	}
	if repo.attributions != 0 {
		t.Fatalf("attributions = %d, want 0", repo.attributions)
	}
}

func TestAttributeRefusesSelfReferralBeforeWriting(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)
	link, err := svc.Link(context.Background(), 1)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	code := link[strings.LastIndex(link, "/")+1:]

	if err := svc.Attribute(context.Background(), 1, code); err != nil {
		t.Fatalf("Attribute: %v — self-referral is a no-op, not a failure", err)
	}
	if repo.attributions != 0 {
		t.Fatalf("attributions = %d, want 0 — the check must not rely on the table's "+
			"constraint, which would make the common case an error", repo.attributions)
	}
}

func TestDiscountPrefersTheLargerPercentage(t *testing.T) {
	repo := newFakeRepo()
	repo.codes["ZZTEST10"] = 10
	svc := newTestService(repo)
	if _, err := svc.Redeem(context.Background(), 5, "ZZTEST10"); err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	repo.pending[5] = true // also invited

	d, err := svc.Discount(context.Background(), 5)
	if err != nil {
		t.Fatalf("Discount: %v", err)
	}
	if d.Percent != InvitePercent {
		t.Fatalf("percent = %d, want %d — one coupon per session, so the buyer gets the "+
			"better of the two rather than the one they happened to redeem", d.Percent, InvitePercent)
	}
	if d.Source != SourceInvite {
		t.Fatalf("source = %q, want %q", d.Source, SourceInvite)
	}
}

func TestDiscountIsZeroForAnOrdinaryAccount(t *testing.T) {
	svc := newTestService(newFakeRepo())

	d, err := svc.Discount(context.Background(), 9)
	if err != nil {
		t.Fatalf("Discount: %v", err)
	}
	if d.Percent != 0 || d.Source != "" {
		t.Fatalf("Discount = %+v, want the zero value — an account with no code and no "+
			"invite must produce the request checkout makes today", d)
	}
}
