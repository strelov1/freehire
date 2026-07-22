//go:build integration

package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/pii"
)

// noopPIIDetector reports no spans, so the fit chain runs with an empty (pass-through)
// redactor: Build succeeds, masking is a no-op, and these tests exercise the analysis path
// exactly as before PII masking (their sample CVs carry no PII to mask).
type noopPIIDetector struct{}

func (noopPIIDetector) Detect(context.Context, string) ([]pii.Span, error) { return nil, nil }
