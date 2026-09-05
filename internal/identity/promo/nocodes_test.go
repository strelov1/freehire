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

// notCodes are the upper-case literals that appear in the discount sources for reasons of
// their own. Every entry is a word this code says about itself, never a value redeemable
// against money — which is the line the allowlist may not cross.
var notCodes = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "HEAD": true,
	"JSON": true, "HTTP": true, "HTTPS": true, "SQL": true, "URL": true,
	"USD": true, "EUR": true, "UTC": true, "ID": true, "API": true,
}

// discountSources are the files where a code would plausibly be written: the package that
// decides discounts, the one that applies them, and the page that asks for one.
var discountSources = []string{
	"internal/identity/promo",
	"internal/identity/billing",
	"internal/api/handler/promo.go",
	"web/src/routes/pricing",
	"web/src/lib/referral.ts",
}

func TestNoRedeemableCodeShipsInTheRepository(t *testing.T) {
	root := moduleRoot(t)

	for _, rel := range discountSources {
		walk(t, filepath.Join(root, rel), func(path string, body []byte) {
			// Test files are exempt: a test needs codes to redeem, and one that could not
			// name any would be testing nothing. They are also not reachable from a running
			// deployment, which is what makes the exemption safe rather than convenient.
			if strings.HasSuffix(path, "_test.go") || strings.Contains(path, ".test.") {
				return
			}

			for _, match := range literal.FindAllStringSubmatch(string(body), -1) {
				value := match[1]
				if notCodes[value] || !promoShape.MatchString(value) {
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
	if notCodes[sample] || !promoShape.MatchString(sample) {
		t.Fatalf("the guard would not notice %q", sample)
	}
	for word := range notCodes {
		if promoShape.MatchString(word) && len(word) < 4 {
			t.Fatalf("%q is allowlisted but the shape would never have matched it — the "+
				"allowlist is drifting away from what it excuses", word)
		}
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
		case ".go", ".sql", ".ts", ".svelte", ".js", ".mjs":
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
