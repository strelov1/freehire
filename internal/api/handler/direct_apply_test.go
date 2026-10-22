package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/ingest/directapply"
)

// Each refusal answers with its own status and its own text; a delivery failure answers
// with ErrDelivery's text only, never the transport's, which can name the employer.
func TestDirectApplyError(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{fmt.Errorf("%w: %q needs an answer", directapply.ErrInvalid, "Stack"), fiber.StatusBadRequest},
		{directapply.ErrNotFound, fiber.StatusNotFound},
		{directapply.ErrUnverified, fiber.StatusForbidden},
		{directapply.ErrClosed, fiber.StatusGone},
		{directapply.ErrAlreadyApplied, fiber.StatusConflict},
		{directapply.ErrInProgress, fiber.StatusConflict},
		{directapply.ErrDailyLimit, fiber.StatusTooManyRequests},
		{fmt.Errorf("%w: %w", directapply.ErrDelivery, errors.New("ses: rejected hr@acme.test")), fiber.StatusBadGateway},
	}
	for _, tc := range cases {
		var fe *fiber.Error
		if !errors.As(directApplyError(tc.err), &fe) {
			t.Fatalf("%v: not a fiber error", tc.err)
		}
		if fe.Code != tc.status {
			t.Errorf("%v: status = %d, want %d", tc.err, fe.Code, tc.status)
		}
		if strings.Contains(fe.Message, "acme.test") || strings.HasPrefix(fe.Message, "invalid application") {
			t.Errorf("%v: message %q leaks the cause or the sentinel prefix", tc.err, fe.Message)
		}
	}
}

// The uploaded CV goes out as what it is: a PDF as cv.pdf, pasted text as cv.txt.
func TestDirectApplyCVs_SendsTheUploadedCVAsItIs(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		data            []byte
		filename, ctype string
	}{
		"a PDF":       {[]byte("%PDF-1.7 binary"), "cv.pdf", "application/pdf"},
		"pasted text": {[]byte("Go developer, 8 years"), "cv.txt", "text/plain; charset=utf-8"},
	} {
		t.Run(name, func(t *testing.T) {
			store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
			if _, err := store.Put(ctx, 7, tc.ctype, tc.data); err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := directApplyCVs{resumes: store}.Attachment(ctx, 7, 11, directapply.CVChoice{Resume: true})
			if err != nil {
				t.Fatalf("Attachment: %v", err)
			}
			if got.Filename != tc.filename || got.ContentType != tc.ctype || string(got.Content) != string(tc.data) {
				t.Errorf("attachment = %s %s %q", got.Filename, got.ContentType, got.Content)
			}
		})
	}

	empty := directApplyCVs{resumes: resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})}
	if _, err := empty.Attachment(ctx, 7, 11, directapply.CVChoice{Resume: true}); !errors.Is(err, directapply.ErrNotFound) {
		t.Errorf("no uploaded CV: err = %v, want ErrNotFound", err)
	}
}
