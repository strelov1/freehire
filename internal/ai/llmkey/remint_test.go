package llmkey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A credential the gateway refused is blocked, not deleted (Forget), so its name stays taken.
// Minting the account a fresh one under the same name then answered 409 on every model call
// for good — measured on production 2026-10-25 — and the account ran on the service credential
// with neither attribution nor its budget. A taken name is retried once under a suffixed one
// that still starts with the account's own name, which is what spend reports group by.
func TestMintRetriesATakenNameUnderASuffixedOne(t *testing.T) {
	var names []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == templatePath {
			_, _ = io.WriteString(w, templateBody)
			return
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		name, _ := body["name"].(string)
		names = append(names, name)
		if name == "freehire-user-42" {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":"virtual key with this name already exists"}`)
			return
		}
		_, _ = io.WriteString(w, mintedBody)
	}))
	t.Cleanup(srv.Close)

	cred, err := configured(srv.URL).Mint(context.Background(), 42)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if cred.Secret != "sk-bf-minted" {
		t.Errorf("Mint = %+v, want the credential minted under the second name", cred)
	}
	if len(names) != 2 || names[0] != "freehire-user-42" || !strings.HasPrefix(names[1], "freehire-user-42-") {
		t.Errorf("names tried = %q, want the plain name then one starting freehire-user-42-", names)
	}
}
