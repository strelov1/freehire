package problemreport

import (
	"errors"
	"strings"
	"testing"
)

func sampleInput() Input {
	return Input{Category: "tool_error", Goal: "Save a job", Expected: "Job saved", Actual: "Operation timed out", ToolName: "save_job", ErrorCode: "deadline_exceeded"}
}

func TestPrepareSanitizesAndDeduplicatesSummaries(t *testing.T) {
	in := sampleInput()
	in.Actual = "Bearer abc123 contact a.person@example.com https://user:pass@example.com/path?token=secret#fragment fhk_abcdef eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyIjoxfQ.signature"
	got, fingerprint, err := prepare(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"abc123", "a.person@example.com", "user:pass", "token=secret", "fragment", "fhk_abcdef", "eyJ"} {
		if strings.Contains(got.Actual, secret) {
			t.Errorf("stored summary contains %q: %s", secret, got.Actual)
		}
	}
	if !strings.Contains(got.Actual, "https://example.com/path") {
		t.Fatalf("URL lost useful path: %s", got.Actual)
	}
	in.Goal = "  SAVE   a job  "
	_, repeated, err := prepare(in)
	if err != nil || repeated != fingerprint {
		t.Fatalf("equivalent summaries do not deduplicate: %v", err)
	}
	in.Expected = "A different result"
	_, changed, err := prepare(in)
	if err != nil || changed == fingerprint {
		t.Fatalf("different reports deduplicated: %v", err)
	}
}

func TestPrepareRedactsCredentialsAndPrivateKeys(t *testing.T) {
	for _, value := range []string{
		"api_key=secretvalue", "password: secretvalue", "refresh_token='secretvalue'",
		`{"password":"secretvalue"}`, `{"api_key":"secretvalue"}`,
		`{"authorization":"Bearer secretvalue"}`, "Authorization: Bearer secretvalue",
		`{"authorization":"Basic secretvalue"}`, "Authorization: Basic secretvalue",
		`{"password":"secretvalue contains spaces"}`, `password='secretvalue contains spaces'`,
		"sk-secretvalue", "ghp_secretvalue", "-----BEGIN PRIVATE KEY-----\nsecretvalue\n-----END PRIVATE KEY-----",
	} {
		in := sampleInput()
		in.Actual = value
		got, _, err := prepare(in)
		if err != nil || strings.Contains(got.Actual, "secretvalue") {
			t.Fatalf("credential not redacted: input %q got %+v err %v", value, got, err)
		}
	}
}

func TestPrepareRefusesInvalidReports(t *testing.T) {
	cases := []Input{sampleInput(), sampleInput(), sampleInput(), sampleInput(), sampleInput(), sampleInput()}
	cases[0].Category = "vacancy"
	cases[1].Goal = " "
	cases[2].Expected = strings.Repeat("界", 1001)
	cases[3].Actual = strings.Repeat("x", 2001)
	cases[4].ToolName = "fhk_secretvalue"
	cases[5].ErrorCode = "an error with private text"
	for _, in := range cases {
		if _, _, err := prepare(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid report accepted: %+v %v", in, err)
		}
	}
	in := sampleInput()
	in.Goal = "  " + strings.Repeat("界", 500) + "  "
	if _, _, err := prepare(in); err != nil {
		t.Fatalf("trimmed unicode boundary rejected: %v", err)
	}
}

func TestRecordFailsClosedWithoutStorageOrIdentity(t *testing.T) {
	var service *Service
	if _, err := service.Record(t.Context(), 0, sampleInput()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing identity: %v", err)
	}
	if receipt, err := service.Record(t.Context(), 42, sampleInput()); err == nil || receipt.ID != 0 {
		t.Fatalf("claimed success without persistence: %+v %v", receipt, err)
	}
}
