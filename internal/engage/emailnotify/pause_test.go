package emailnotify

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func redisURL(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	return mr, "redis://" + mr.Addr() + "/0"
}

// held reports whether a class or all-mail key would hold binary at worker start.
func held(t *testing.T, url, binary string) bool {
	t.Helper()
	return EmailHeld(context.Background(), url, binary)
}

func TestClassKeysHoldExactlyTheirMembers(t *testing.T) {
	want := map[Class][]string{
		ClassTransactional: {"pro-welcome-mail"},
		ClassEngagement:    {"notify", "message-notify", "mentorship-remind"},
		ClassLifecycle:     {"onboarding", "nudge", "remind", "limit-nudge-mail"},
	}
	for class, members := range want {
		mr, url := redisURL(t)
		if err := mr.Set("freehire:pause:mail:"+string(class), "1"); err != nil {
			t.Fatal(err)
		}
		in := map[string]bool{}
		for _, m := range members {
			in[m] = true
			if !held(t, url, m) {
				t.Errorf("%s: %s not held", class, m)
			}
		}
		for other := range workerClass {
			if !in[other] && held(t, url, other) {
				t.Errorf("%s hold also holds %s", class, other)
			}
		}
		if held(t, url, "server") || held(t, url, "ingest") {
			t.Errorf("%s hold reaches a worker outside the table", class)
		}
	}
}

func TestAllMailKeyHoldsEveryMailWorkerOnly(t *testing.T) {
	mr, url := redisURL(t)
	if err := mr.Set("freehire:pause:mail:all", "1"); err != nil {
		t.Fatal(err)
	}
	for binary := range workerClass {
		if !held(t, url, binary) {
			t.Errorf("%s not held by mail:all", binary)
		}
	}
	if held(t, url, "ingest") {
		t.Error("mail:all holds a worker that sends no mail")
	}
}

func TestIgnorePauseLiftsClassHolds(t *testing.T) {
	mr, url := redisURL(t)
	_ = mr.Set("freehire:pause:mail:all", "1")
	t.Setenv("FREEHIRE_IGNORE_PAUSE", "1")
	if held(t, url, "notify") {
		t.Error("held despite FREEHIRE_IGNORE_PAUSE")
	}
}

func TestUnreadableRedisRunsTheWorker(t *testing.T) {
	mr, url := redisURL(t)
	_ = mr.Set("freehire:pause:mail:all", "1")
	mr.Close()
	if held(t, url, "notify") {
		t.Error("held while Redis is unreadable, want fail-open")
	}
}

func TestHoldKeysForUnclassifiedBinaryIsEmpty(t *testing.T) {
	for _, b := range []string{"server", "broadcast", "mail-preview", "ingest"} {
		if k := HoldKeys(b); len(k) != 0 {
			t.Errorf("HoldKeys(%q) = %v, want none: no class may touch it", b, k)
		}
	}
}

// A new worker that builds an SES client for mail must pick a class, or an
// operator's hold silently skips it.
func TestEveryMailWorkerIsClassified(t *testing.T) {
	notHeld := map[string]bool{"broadcast": true} // hand-run; see workerClass
	dirs, err := filepath.Glob("../../../cmd/*/*.go")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cmd sources found: %v", err)
	}
	var missing []string
	for _, f := range dirs {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "emailnotify.NewClient(") {
			continue
		}
		bin := filepath.Base(filepath.Dir(f))
		if _, ok := workerClass[bin]; !ok && !notHeld[bin] {
			missing = append(missing, bin)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("binaries build an SES client but have no mail class: %v", missing)
	}
}

func TestNoClassCoversTheAPIServersMail(t *testing.T) {
	for _, b := range []string{"server", "api"} {
		if _, ok := ClassOf(b); ok {
			t.Errorf("%s is in the class table; codes, resets and application mail are never held", b)
		}
	}
}
