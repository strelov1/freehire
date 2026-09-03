package main

import (
	"context"

	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/application/autoapply"
)

// assemblerAnswerSource adapts *candidateprofile.Assembler to autoapply.AnswerSource. The
// assembler is exactly what internal/handler's extension-autofill path already resolves a
// candidate's profile through (see internal/candidateprofile's package doc) — this worker
// intentionally reads no other source, so a form asking beyond identity/work-authorization
// always parks rather than being answered from somewhere the candidate never confirmed.
type assemblerAnswerSource struct {
	assembler *candidateprofile.Assembler
}

var _ autoapply.AnswerSource = assemblerAnswerSource{}

func (a assemblerAnswerSource) Answers(ctx context.Context, userID int64) (map[string]string, error) {
	profile, err := a.assembler.Assemble(ctx, userID)
	if err != nil {
		return nil, err
	}
	return profile.Fields(), nil
}
