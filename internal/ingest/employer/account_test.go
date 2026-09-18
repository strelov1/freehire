package employer

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// fakeRepo is an in-memory Repository double: one account per user, one slug reservation
// per company, and a tiny companies table of its own.
type fakeRepo struct {
	accounts       map[int64]Account
	slugOwner      map[string]int64 // company_slug -> user_id, for the unique-constraint stand-in
	aliases        map[string]string
	companies      map[string]struct{ name, website string }
	profilePatches []CompanyProfilePatch
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		accounts:  map[int64]Account{},
		slugOwner: map[string]int64{},
		aliases:   map[string]string{},
		companies: map[string]struct{ name, website string }{},
	}
}

func (r *fakeRepo) InsertPending(_ context.Context, userID int64, companySlug, companyName, workEmail string) (Account, error) {
	if _, ok := r.accounts[userID]; ok {
		return Account{}, ErrAlreadyHasAccount
	}
	if _, ok := r.slugOwner[companySlug]; ok {
		return Account{}, ErrCompanyAlreadyClaimed
	}
	acc := Account{UserID: userID, CompanySlug: companySlug, CompanyName: companyName, WorkEmail: workEmail, Status: StatusPending}
	r.accounts[userID] = acc
	r.slugOwner[companySlug] = userID
	return acc, nil
}

func (r *fakeRepo) GetByUserID(_ context.Context, userID int64) (Account, error) {
	acc, ok := r.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	return acc, nil
}

func (r *fakeRepo) ListPending(_ context.Context) ([]Account, error) {
	var out []Account
	for _, acc := range r.accounts {
		if acc.Status == StatusPending {
			out = append(out, acc)
		}
	}
	return out, nil
}

func (r *fakeRepo) Activate(_ context.Context, userID int64) (Account, error) {
	acc, ok := r.accounts[userID]
	if !ok || acc.Status != StatusPending {
		return Account{}, ErrClaimNotPending
	}
	acc.Status = StatusActive
	r.accounts[userID] = acc
	return acc, nil
}

func (r *fakeRepo) Revoke(_ context.Context, userID int64) (Account, error) {
	acc, ok := r.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	acc.Status = StatusRevoked
	r.accounts[userID] = acc
	return acc, nil
}

func (r *fakeRepo) DeletePending(_ context.Context, userID int64) error {
	acc, ok := r.accounts[userID]
	if !ok || acc.Status != StatusPending {
		return ErrClaimNotPending
	}
	delete(r.accounts, userID)
	delete(r.slugOwner, acc.CompanySlug)
	return nil
}

func (r *fakeRepo) ResolveCanonicalSlug(_ context.Context, candidate string) (string, error) {
	if canon, ok := r.aliases[candidate]; ok {
		return canon, nil
	}
	return candidate, nil
}

func (r *fakeRepo) ExistingCompany(_ context.Context, slug string) (string, string, bool, error) {
	c, ok := r.companies[slug]
	if !ok {
		return "", "", false, nil
	}
	return c.name, c.website, true, nil
}

// UpdateCompanyProfile records the applied patch (for assertions) and, for the one field
// this fake's tiny companies map also models, updates it — the real repository's own
// column-by-column mapping is covered by the sqlc-generated integration path instead.
func (r *fakeRepo) UpdateCompanyProfile(_ context.Context, slug string, patch CompanyProfilePatch) error {
	r.profilePatches = append(r.profilePatches, patch)
	if patch.Website != nil {
		c := r.companies[slug]
		c.website = *patch.Website
		r.companies[slug] = c
	}
	return nil
}

func (r *fakeRepo) SeedCompanyWebsite(_ context.Context, slug, name, website string) error {
	c := r.companies[slug]
	if c.website != "" {
		return nil
	}
	if c.name == "" {
		c.name = name
	}
	c.website = website
	r.companies[slug] = c
	return nil
}

// fakeCodeIssuer stands in for *accounts.Service's IssueCode/ConfirmCode.
type fakeCodeIssuer struct {
	issued    map[string]string // "purpose:userID" -> code
	confirmed []string
	failNext  error
}

func newFakeCodeIssuer() *fakeCodeIssuer { return &fakeCodeIssuer{issued: map[string]string{}} }

func codeIssuerKey(userID int64, purpose string) string {
	return fmt.Sprintf("%s:%d", purpose, userID)
}

func (f *fakeCodeIssuer) IssueCode(ctx context.Context, userID int64, purpose, email string, send func(ctx context.Context, email, code string) error) error {
	if f.failNext != nil {
		return f.failNext
	}
	code := "654321"
	f.issued[codeIssuerKey(userID, purpose)] = code
	return send(ctx, email, code)
}

func (f *fakeCodeIssuer) ConfirmCode(_ context.Context, userID int64, purpose, code string) error {
	want, ok := f.issued[codeIssuerKey(userID, purpose)]
	if !ok || want != code {
		return errors.New("invalid code")
	}
	f.confirmed = append(f.confirmed, codeIssuerKey(userID, purpose))
	delete(f.issued, codeIssuerKey(userID, purpose))
	return nil
}

// fakeClaimMailer records what was mailed, standing in for the real transport.
type fakeClaimMailer struct {
	sent []struct{ email, code string }
}

func (m *fakeClaimMailer) SendClaimVerificationCode(_ context.Context, email, code string) error {
	m.sent = append(m.sent, struct{ email, code string }{email, code})
	return nil
}

func TestClaim_ResolvesThroughAnAlias(t *testing.T) {
	repo := newFakeRepo()
	repo.aliases["acme-corp"] = "acme"
	repo.companies["acme"] = struct{ name, website string }{name: "Acme Inc.", website: ""}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	acc, err := s.Claim(context.Background(), 1, "Acme Corp", "hr@acme.test")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if acc.CompanySlug != "acme" {
		t.Errorf("company_slug = %q, want the canonical alias target", acc.CompanySlug)
	}
	if acc.CompanyName != "Acme Inc." {
		t.Errorf("company_name = %q, want the EXISTING company's stored name, not the typed one", acc.CompanyName)
	}
}

func TestClaim_MintsANewSlugForAnUnknownCompany(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	// "Co" is a legal-form token normalize.CompanySlug strips (see
	// internal/dict/normalize/company.go, docs/agents/company-identity.md) — "Brand New Co"
	// slugs the same as "Brand New", which is the point of sharing that exact function with
	// ingest rather than a bespoke one here.
	acc, err := s.Claim(context.Background(), 1, "Brand New Co", "hr@brandnew.test")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if acc.CompanySlug != "brand-new" {
		t.Errorf("company_slug = %q, want a freshly normalized slug", acc.CompanySlug)
	}
	if acc.CompanyName != "Brand New Co" {
		t.Errorf("company_name = %q, want the claimant's own typed name for a brand-new company", acc.CompanyName)
	}
}

func TestClaim_MailsAVerificationCodeToTheWorkEmail(t *testing.T) {
	repo := newFakeRepo()
	mailer := &fakeClaimMailer{}
	s := New(repo, newFakeCodeIssuer(), mailer, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].email != "hr@acme.test" {
		t.Fatalf("sent = %+v, want one code to hr@acme.test", mailer.sent)
	}
}

func TestClaim_RejectsAnEmptyCompanyNameOrWorkEmail(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "  ", "hr@acme.test"); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid for a blank company name", err)
	}
	if _, err := s.Claim(context.Background(), 1, "Acme", "  "); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid for a blank work email", err)
	}
}

func TestClaim_RejectsAPublicWebmailDomain(t *testing.T) {
	repo := newFakeRepo()
	mailer := &fakeClaimMailer{}
	s := New(repo, newFakeCodeIssuer(), mailer, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@gmail.com"); !errors.Is(err, ErrPublicWebmailDomain) {
		t.Fatalf("err = %v, want ErrPublicWebmailDomain", err)
	}
	if len(mailer.sent) != 0 {
		t.Error("a refused claim must mail nothing")
	}
	if _, err := repo.GetByUserID(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Error("a refused claim must not reserve a slug")
	}
}

func TestClaim_WithNoMailerConfiguredReportsErrMailUnavailable(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), nil, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); !errors.Is(err, ErrMailUnavailable) {
		t.Fatalf("err = %v, want ErrMailUnavailable", err)
	}
	if _, err := repo.GetByUserID(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Error("a refused claim (no mailer) must not reserve a slug")
	}
}

func TestClaim_RefusesASecondClaimByTheSameUser(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if _, err := s.Claim(context.Background(), 1, "Other Co", "hr@other.test"); !errors.Is(err, ErrAlreadyHasAccount) {
		t.Fatalf("err = %v, want ErrAlreadyHasAccount", err)
	}
}

func TestClaim_RefusesASecondClaimOnAnAlreadyClaimedCompany(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if _, err := s.Claim(context.Background(), 2, "Acme", "founder@acme.test"); !errors.Is(err, ErrCompanyAlreadyClaimed) {
		t.Fatalf("err = %v, want ErrCompanyAlreadyClaimed", err)
	}
}

func TestConfirmClaim_ActivatesOnAMatchingDomain(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	codes := newFakeCodeIssuer()
	s := New(repo, codes, &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	acc, err := s.ConfirmClaim(context.Background(), 1, "654321")
	if err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	if acc.Status != StatusActive {
		t.Errorf("status = %q, want active", acc.Status)
	}
}

func TestConfirmClaim_StaysPendingOnAMismatchedDomain(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	acc, err := s.ConfirmClaim(context.Background(), 1, "654321")
	if err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	if acc.Status != StatusPending {
		t.Errorf("status = %q, want pending — a mismatched domain must not auto-activate", acc.Status)
	}
}

func TestConfirmClaim_StaysPendingOnAnUnknownWebsite(t *testing.T) {
	repo := newFakeRepo() // no companies row at all: website unknown
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Brand New", "hr@brandnew.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	acc, err := s.ConfirmClaim(context.Background(), 1, "654321")
	if err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	if acc.Status != StatusPending {
		t.Errorf("status = %q, want pending — an unknown website must not auto-activate", acc.Status)
	}
}

func TestConfirmClaim_RejectsAWrongCode(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "000000"); err == nil {
		t.Fatal("ConfirmClaim: want an error for a wrong code")
	}
	acc, _ := repo.GetByUserID(context.Background(), 1)
	if acc.Status != StatusPending {
		t.Errorf("status = %q, want pending after a wrong code", acc.Status)
	}
}

func TestApproveClaim_ActivatesAndSeedsABlankWebsite(t *testing.T) {
	repo := newFakeRepo() // no companies row: website unknown, so ConfirmClaim left it pending
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Brand New", "hr@brandnew.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	acc, err := s.ApproveClaim(context.Background(), 1)
	if err != nil {
		t.Fatalf("ApproveClaim: %v", err)
	}
	if acc.Status != StatusActive {
		t.Errorf("status = %q, want active", acc.Status)
	}
	_, website, found, _ := repo.ExistingCompany(context.Background(), "brand-new")
	if !found || website != "brandnew.test" {
		t.Errorf("website = %q (found=%v), want the confirmed work-email domain seeded by moderator approval", website, found)
	}
}

func TestApproveClaim_NeverOverwritesAnExistingWebsite(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@subsidiary.acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil { // mismatched domain -> stays pending
		t.Fatalf("ConfirmClaim: %v", err)
	}
	if _, err := s.ApproveClaim(context.Background(), 1); err != nil {
		t.Fatalf("ApproveClaim: %v", err)
	}
	_, website, _, _ := repo.ExistingCompany(context.Background(), "acme")
	if website != "https://acme.test" {
		t.Errorf("website = %q, must never be overwritten by an approval", website)
	}
}

func TestRejectClaim_FreesTheSlug(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := s.RejectClaim(context.Background(), 1); err != nil {
		t.Fatalf("RejectClaim: %v", err)
	}
	if _, err := repo.GetByUserID(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Error("a rejected claim must be gone")
	}
	if _, err := s.Claim(context.Background(), 2, "Acme", "founder@acme.test"); err != nil {
		t.Errorf("Claim by a different user on the freed slug: %v", err)
	}
}

func TestRevokeAccount_KeepsTheSlugReserved(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.RevokeAccount(context.Background(), 1); err != nil {
		t.Fatalf("RevokeAccount: %v", err)
	}
	if _, err := s.Claim(context.Background(), 2, "Acme", "founder@acme.test"); !errors.Is(err, ErrCompanyAlreadyClaimed) {
		t.Errorf("err = %v, want ErrCompanyAlreadyClaimed — a revoked slug must not be self-service claimable", err)
	}
}

func TestUpdateCompanyProfile_AppliesThePatchForAnActiveAccount(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	tagline := "We build things"
	if _, err := s.UpdateCompanyProfile(context.Background(), 1, CompanyProfilePatch{Tagline: &tagline}); err != nil {
		t.Fatalf("UpdateCompanyProfile: %v", err)
	}
	if len(repo.profilePatches) != 1 || repo.profilePatches[0].Tagline == nil || *repo.profilePatches[0].Tagline != tagline {
		t.Errorf("profilePatches = %+v, want one patch carrying the tagline", repo.profilePatches)
	}
}

func TestUpdateCompanyProfile_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	tagline := "We build things"
	if _, err := s.UpdateCompanyProfile(context.Background(), 1, CompanyProfilePatch{Tagline: &tagline}); !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
	if len(repo.profilePatches) != 0 {
		t.Error("a refused edit must not reach the repository")
	}
}

func TestMyAccount_ReturnsAPendingAccountWithoutRefusing(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	acc, err := s.MyAccount(context.Background(), 1)
	if err != nil {
		t.Fatalf("MyAccount: %v, want no error for a pending account", err)
	}
	if acc.Status != StatusPending {
		t.Errorf("status = %q, want pending", acc.Status)
	}
}

func TestMyAccount_ReportsErrNotFoundForNoAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.MyAccount(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestActiveAccount_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ActiveAccount(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Errorf("err = %v, want ErrNotActive for a pending account", err)
	}
}

func TestActiveAccount_RefusesARevokedAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.RevokeAccount(context.Background(), 1); err != nil {
		t.Fatalf("RevokeAccount: %v", err)
	}
	if _, err := s.ActiveAccount(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Errorf("err = %v, want ErrNotActive for a revoked account", err)
	}
}

func TestActiveAccount_ReturnsAnActiveAccount(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	acc, err := s.ActiveAccount(context.Background(), 1)
	if err != nil {
		t.Fatalf("ActiveAccount: %v", err)
	}
	if acc.CompanySlug != "acme" {
		t.Errorf("company_slug = %q, want acme", acc.CompanySlug)
	}
}
