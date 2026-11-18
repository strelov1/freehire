package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// jevAnswering stands in for the Jev API, recording the request and answering one score.
func jevAnswering(t *testing.T, status int, body string, got *map[string]any) *JevJudge {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer jev-key" {
			t.Errorf("request %s with auth %q, want /v1/systemone with the key", r.URL.Path, r.Header.Get("Authorization"))
		}
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewJevJudge("jev-key", jev.WithBaseURL(srv.URL))
}

func TestJevJudgeAsksOneYesNoQuestionAboutThePost(t *testing.T) {
	var req map[string]any
	j := jevAnswering(t, http.StatusOK, `{"answers":{"vacancy":{"type":"noul","noul":0.97}}}`, &req)

	ok, err := j.IsVacancy(context.Background(), "RUST DEVELOPER | REMOTE")
	if err != nil || !ok {
		t.Fatalf("IsVacancy = %v, %v; want true", ok, err)
	}
	if req["state"].(map[string]any)["post"] != "RUST DEVELOPER | REMOTE" {
		t.Errorf("state = %v, want the post text", req["state"])
	}
	q := req["questions"].(map[string]any)["vacancy"].(map[string]any)
	if q["type"] != "noul" {
		t.Errorf("question type = %v, want noul", q["type"])
	}
}

func TestJevJudgeDeclinesALowScore(t *testing.T) {
	j := jevAnswering(t, http.StatusOK, `{"answers":{"vacancy":{"type":"noul","noul":0.18}}}`, nil)
	if ok, err := j.IsVacancy(context.Background(), "events digest"); err != nil || ok {
		t.Fatalf("IsVacancy = %v, %v; want false", ok, err)
	}
}

// An answer that is not an answer is an error, never a "no": a "no" stamps the post done
// for good, while an error admits it for the extractor (see judgeRescues).
func TestJevJudgeTreatsAMalformedAnswerAsAnError(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"server error":   {http.StatusServiceUnavailable, `{"detail":"down"}`},
		"missing answer": {http.StatusOK, `{"answers":{}}`},
		"missing score":  {http.StatusOK, `{"answers":{"vacancy":{"type":"noul"}}}`},
	} {
		t.Run(name, func(t *testing.T) {
			j := jevAnswering(t, tc.status, tc.body, nil)
			if _, err := j.IsVacancy(context.Background(), "x"); err == nil {
				t.Fatal("IsVacancy returned no error")
			}
		})
	}
}

func TestJevJudgeCapsTheTextItSends(t *testing.T) {
	var req map[string]any
	j := jevAnswering(t, http.StatusOK, `{"answers":{"vacancy":{"type":"noul","noul":0.9}}}`, &req)
	if _, err := j.IsVacancy(context.Background(), strings.Repeat("я", jevMaxRunes+500)); err != nil {
		t.Fatalf("IsVacancy: %v", err)
	}
	if got := len([]rune(req["state"].(map[string]any)["post"].(string))); got != jevMaxRunes {
		t.Errorf("sent %d runes, want %d", got, jevMaxRunes)
	}
}

// No key is today's behaviour: the runners see no judge at all.
func TestJudgeFromKeyWithoutAKeyIsNoJudge(t *testing.T) {
	if j := JudgeFromKey(""); j != nil {
		t.Fatalf("JudgeFromKey(\"\") = %v, want a nil interface", j)
	}
	if JudgeFromKey("k") == nil {
		t.Fatal("JudgeFromKey(\"k\") = nil, want a judge")
	}
}
