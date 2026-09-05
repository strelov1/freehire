package promo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This repository is public. A promo code readable from the source is a promo code that
// gets drained, and the same is true of anything that CREATES one — a seed script, a
// fixture inserted at startup, a "temporary" fallback. Two guards, because the two failures
// look nothing alike.
//
// Neither is a substitute for judgement, and neither pretends to be: a code obfuscated on
// purpose defeats both. They catch the accident, which is the case that actually happens.

// promoShape is what promo_codes.code accepts. Deliberately the same expression as the
// service's, so a guard cannot pass by being narrower than the thing it guards.
var promoShape = regexp.MustCompile(`^[A-Z0-9]{4,32}$`)

// literal finds double-quoted strings, which is where a code would be.
var literal = regexp.MustCompile(`"([^"\n]{4,32})"`)

// fixturePrefix marks a code that exists only to be redeemed by a test. Tests need codes —
// one that could name none would be testing nothing — so they carry a prefix no offer will
// ever use, and the guard covers test files like everything else. The alternative, exempting
// test files, leaves the largest body of code-shaped literals unwatched.
const fixturePrefix = "ZZ"

// notCodes are the upper-case literals that appear in the discount sources and its documents
// for reasons of their own. Every entry is a word this feature says about itself, never a
// value redeemable against money — which is the line the allowlist may not cross.
var notCodes = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "HEAD": true,
	"JSON": true, "HTTP": true, "HTTPS": true, "SQL": true, "URL": true,
	"USD": true, "EUR": true, "UTC": true, "ID": true, "API": true,
	"NULL": true, "TRUE": true, "FALSE": true, "CHECK": true, "UNIQUE": true,
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"SHALL": true, "WHEN": true, "THEN": true, "AND": true, "OR": true,
}

// discountSources are the files where a code would plausibly be written: the package that
// decides discounts, the one that applies them, the page that asks for one, the migration
// that creates the tables, and the documents that describe all of it.
//
// The documents are in the list because they are the easiest place to leak a code and the
// least likely to be reviewed for it — "insert EARLY90 when the offer starts" reads like
// instructions and ships like a credential.
var discountSources = []string{
	"internal/identity/promo",
	// The discount files of billing, and not the whole package. The rest of it speaks a
	// provider vocabulary full of upper-case words — event types, SQLSTATEs — and
	// allowlisting them one at a time would grow an exception list that eventually excuses
	// a real code. A promo code would be written where discounts are, and that is here.
	"internal/identity/billing/discount.go",
	"internal/identity/billing/discount_test.go",
	"internal/identity/billing/credit_integration_test.go",
	"internal/api/handler/promo.go",
	"internal/api/handler/promo_test.go",
	"web/src/routes/pricing",
	"web/src/lib/referral.ts",
	"web/src/lib/referral.test.ts",
	"migrations/0140_promo_and_invites.sql",
	"openspec/specs/promo-codes",
	"openspec/specs/invite-referrals",
	"openspec/changes/archive/2026-09-05-add-invite-and-promo-discounts",
}

func TestNoRedeemableCodeShipsInTheRepository(t *testing.T) {
	root := moduleRoot(t)

	for _, rel := range discountSources {
		walk(t, filepath.Join(root, rel), func(path string, body []byte) {
			// This file is the exception, and only this one: it has to name a code-shaped
			// value to prove the guard can still see one.
			if strings.HasSuffix(path, "nocodes_test.go") {
				return
			}

			for _, match := range literal.FindAllStringSubmatch(string(body), -1) {
				value := match[1]
				if notCodes[value] || strings.HasPrefix(value, fixturePrefix) {
					continue
				}
				if !promoShape.MatchString(value) {
					continue
				}
				t.Errorf("%s carries %q, which promo_codes would accept as a code. "+
					"Codes are inserted by an operator and never written down here — if this "+
					"is not a code, give it a spelling the table could not hold.",
					strings.TrimPrefix(path, root+string(filepath.Separator)), value)
			}
		})
	}
}

func TestNothingInTheRepositoryCreatesAPromoCode(t *testing.T) {
	root := moduleRoot(t)

	// Migrations create the TABLE; nothing anywhere creates a ROW. An INSERT here would be
	// a code path that mints an offer, which is the failure this whole design is arranged
	// around — an operator's INSERT is the only interface, and it is also what makes
	// `active = false` a rollback that needs no deploy.
	for _, dir := range []string{"internal", "cmd", "migrations", "web/src", "scripts"} {
		walk(t, filepath.Join(root, dir), func(path string, body []byte) {
			// Exempt for the same reason as above, plus one of its own: this file names the
			// statement it is looking for, so a guard that read itself would always fail.
			if strings.HasSuffix(path, "_test.go") || strings.Contains(path, ".test.") {
				return
			}
			lowered := strings.ToLower(string(body))
			if !strings.Contains(lowered, "insert into promo_codes") {
				return
			}
			t.Errorf("%s inserts into promo_codes. Offers are created by an operator, not by "+
				"code — see internal/identity/promo/AGENTS.md.",
				strings.TrimPrefix(path, root+string(filepath.Separator)))
		})
	}
}

// A guard that cannot fail is not a guard. This asserts the predicate itself, because the
// two tests above pass on a clean tree either way — and would keep passing if the shape
// were quietly narrowed to match nothing.
func TestTheGuardWouldCatchACode(t *testing.T) {
	// Assembled rather than written, so this file does not contain the thing it forbids.
	sample := "EARLY" + "90"
	if notCodes[sample] || strings.HasPrefix(sample, fixturePrefix) || !promoShape.MatchString(sample) {
		t.Fatalf("the guard would not notice %q", sample)
	}

	// And the fixture prefix must not be a way to smuggle one past it: it is only an
	// exemption because no offer will ever be spelled that way.
	if !strings.HasPrefix(fixturePrefix, "ZZ") {
		t.Fatalf("fixturePrefix = %q — it has to be something an operator would never "+
			"choose for a real offer", fixturePrefix)
	}
}

// moduleRoot walks up from the package directory to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's working directory")
		}
		dir = parent
	}
}

// walk visits every readable source file under root, which may also be a single file.
func walk(t *testing.T, root string, visit func(path string, body []byte)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".svelte-kit" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".sql", ".ts", ".svelte", ".js", ".mjs", ".md":
		default:
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		visit(path, body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
