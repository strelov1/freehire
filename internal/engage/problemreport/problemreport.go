// Package problemreport records concise account-bound failures for operator triage.
package problemreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
)

var ErrInvalid = errors.New("problem report: invalid input")

type Input struct {
	Category  string `json:"category"`
	Goal      string `json:"goal"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
	ToolName  string `json:"tool_name,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type Receipt struct {
	ID              int64     `json:"id"`
	Status          string    `json:"status"`
	RequestCount    int32     `json:"request_count"`
	FirstReportedAt time.Time `json:"first_reported_at"`
	LastReportedAt  time.Time `json:"last_reported_at"`
}

type Service struct {
	queries *db.Queries
}

func New(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

// Record atomically creates an open report or increments its repeat count.
func (s *Service) Record(ctx context.Context, userID int64, input Input) (Receipt, error) {
	if userID <= 0 {
		return Receipt{}, fmt.Errorf("%w: signed-in account required", ErrInvalid)
	}
	in, fingerprint, err := prepare(input)
	if err != nil {
		return Receipt{}, err
	}
	if s == nil || s.queries == nil {
		return Receipt{}, errors.New("problem reporting is unavailable")
	}
	// RequestIDs middleware generates this context value with uuid.NewString();
	// it never trusts the caller's X-Request-ID header.
	requestID := observability.RequestID(ctx)
	row, err := s.queries.RecordProblemReport(ctx, db.RecordProblemReportParams{
		UserID:      userID,
		Category:    in.Category,
		Goal:        in.Goal,
		Expected:    in.Expected,
		Actual:      in.Actual,
		ToolName:    in.ToolName,
		ErrorCode:   in.ErrorCode,
		Fingerprint: fingerprint,
		RequestID:   requestID,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("record problem report: %w", err)
	}
	slog.InfoContext(
		ctx,
		"problem_report_recorded",
		"report_id", row.ID,
		"category", in.Category,
		"request_count", row.RequestCount,
		"request_id", requestID,
	)
	return Receipt{
		ID:              row.ID,
		Status:          row.Status,
		RequestCount:    row.RequestCount,
		FirstReportedAt: row.FirstReportedAt.Time,
		LastReportedAt:  row.LastReportedAt.Time,
	}, nil
}

var (
	emailPattern      = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	urlPattern        = regexp.MustCompile(`(?i)https?://[^\s<>]+`)
	credentialPattern = regexp.MustCompile(
		`(?i)\b(?:bearer\s+|` +
			`(?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|secret|authorization)["']?\s*[=:]\s*)` +
			`(?:"[^"\r\n]*"|'[^'\r\n]*'|(?:(?:bearer|basic)\s+)?[^\s,;"']+)`,
	)
	tokenPattern      = regexp.MustCompile(`\b(?:fh[km]_|sk-|gh[pousr]_)[A-Za-z0-9_-]+|\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,79}$`)
)

func sanitize(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, "[redacted private key]")
	value = urlPattern.ReplaceAllStringFunc(value, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return "[redacted URL]"
		}
		u.User = nil
		u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
		u.ForceQuery = false
		return u.String()
	})
	value = credentialPattern.ReplaceAllString(value, "[redacted credential]")
	value = tokenPattern.ReplaceAllString(value, "[redacted token]")
	value = emailPattern.ReplaceAllString(value, "[redacted email]")
	return strings.Join(strings.Fields(value), " ")
}

func prepare(in Input) (Input, string, error) {
	switch in.Category {
	case "tool_error", "missing_capability", "other":
	default:
		return Input{}, "", fmt.Errorf("%w: unknown category", ErrInvalid)
	}
	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{
		{name: "goal", value: &in.Goal, max: 500},
		{name: "expected", value: &in.Expected, max: 1000},
		{name: "actual", value: &in.Actual, max: 2000},
	} {
		value := strings.TrimSpace(*field.value)
		length := utf8.RuneCountInString(value)
		invalidLength := length == 0 || length > field.max
		if !utf8.ValidString(value) || invalidLength {
			return Input{}, "", fmt.Errorf(
				"%w: %s must contain 1 to %d characters",
				ErrInvalid,
				field.name,
				field.max,
			)
		}
		*field.value = sanitize(value)
		// Redaction can expand a short credential into a longer placeholder.
		if utf8.RuneCountInString(*field.value) > field.max {
			return Input{}, "", fmt.Errorf("%w: sanitized %s is too long", ErrInvalid, field.name)
		}
	}
	for _, field := range []*string{&in.ToolName, &in.ErrorCode} {
		*field = strings.TrimSpace(*field)
		if *field == "" {
			continue
		}
		if !identifierPattern.MatchString(*field) || sanitize(*field) != *field {
			return Input{}, "", fmt.Errorf("%w: tool_name and error_code must be identifiers of up to 80 characters", ErrInvalid)
		}
	}
	// JSON field boundaries avoid ambiguous concatenation; whitespace and case do
	// not split repeated summaries into separate open reports.
	payload, _ := json.Marshal(in)
	sum := sha256.Sum256([]byte(strings.ToLower(string(payload))))
	return in, hex.EncodeToString(sum[:]), nil
}
