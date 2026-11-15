package main

import (
	"context"

	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/application/autoapply"
)

// assemblerAnswerSource adapts *candidateprofile.Assembler to autoapply.AnswerSource. This
// worker reads no other source, so a form asking beyond identity/work-authorization always
// parks rather than being answered from something the candidate never confirmed.
type assemblerAnswerSource struct {
	assembler *candidateprofile.Assembler
}

var _ autoapply.AnswerSource = assemblerAnswerSource{}

func (a assemblerAnswerSource) Answers(ctx context.Context, userID int64) (map[string]string, error) {
	profile, err := a.assembler.Assemble(ctx, userID)
	if err != nil {
		return nil, err
	}
	// FieldsWithBankedAnswers, not Fields: resolving an application form is the one path banked
	// answers belong on (see candidateprofile.Fields).
	return profile.FieldsWithBankedAnswers(), nil
}
