package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/resume"
)

// cvTextMaxRunes and cvTextMinRunes bound the CV add_cv accepts. The ceiling is room for a
// long CV and nothing else; the floor refuses a line that is not a CV, which would spend the
// account's one extraction over this channel on nothing.
const (
	cvTextMaxRunes = 30_000
	cvTextMinRunes = 100
)

// cvSetupMCPTools are the setup tools served to an OAuth grant: adding a first CV and reading
// what setup is missing. Storing a CV (PUT /me/resume) and reading its status (GET /me/resume)
// are cookie-only over REST, so a full-scope API key gets neither.
func (h *assistantHandlers) cvSetupMCPTools(origin string) []assistant.Tool {
	return []assistant.Tool{h.setupStatusTool(origin), h.addCVTool(origin)}
}

type addCVArgs struct {
	Text string `json:"text"`
}

// suggestedProfile is what the CV suggests for the job-search profile, under the profile's own
// field names so a confirmed value is sent back unchanged. The dictionaries never guess, so a
// field they could not resolve is empty.
type suggestedProfile struct {
	Specializations []string `json:"specializations"`
	Seniorities     []string `json:"seniorities"`
	Skills          []string `json:"skills"`
}

type addCVResult struct {
	Stored     bool             `json:"stored"`
	UploadedAt *time.Time       `json:"uploaded_at,omitempty"`
	Suggested  suggestedProfile `json:"suggested"`
	Next       string           `json:"next"`
}

const addCVNext = "Show the person every suggested value, ask which to keep, and save only the ones " +
	"they confirm with update_profile. Their work history is read in the background; skills found " +
	"in it are then added to the profile's skills, and can be removed with update_profile. " +
	"get_setup_status reports when reading is done."

// addCVTool stores a CV the person supplied, as text, for an account that has none, through
// storeFirstCV — the website's own upload use case, so the document, the free background
// extraction and the experience-bank import (cv_import: the text IS the stored CV each atom
// traces to) are the same as a paste on the profile page. It takes only the person's own CV,
// never one an agent composed: a model-written history banked as cv_import would read as the
// candidate's own words. See openspec/changes/mcp-cv-setup/design.md.
func (h *assistantHandlers) addCVTool(origin string) assistant.Tool {
	profileURL := strings.TrimSuffix(origin, "/") + "/my/profile"
	return assistant.Tool{
		Name:   "add_cv",
		Title:  "Add my CV",
		Effect: assistant.EffectAdds,
		Description: "Store the person's own CV, as text, for an account that has no CV yet. Pass the CV " +
			"they supplied verbatim — text they pasted, or the content of a CV file they gave you. Never " +
			"compose, summarise or embellish a CV: it is stored as theirs and its work history is recorded " +
			"as their own words. If they have no CV, they upload or write one on the website instead. " +
			"Between 100 and 30000 characters. Refused when the account already has a CV; it is replaced " +
			"on the website. Returns the profile values the CV suggests (specializations, seniorities, " +
			"skills) for the person to confirm. Once the work history has been read in the background, " +
			"skills found in it are added to the profile's skills, as with an upload on the website.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The person's CV text, verbatim.",
					"minLength":   cvTextMinRunes,
					"maxLength":   cvTextMaxRunes,
				},
			},
			"required":             []string{"text"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in addCVArgs
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			return h.addCV(ctx, userID, strings.TrimSpace(in.Text), profileURL)
		},
	}
}

func (h *assistantHandlers) addCV(ctx context.Context, userID int64, text, profileURL string) (any, error) {
	switch n := utf8.RuneCountInString(text); {
	case n < cvTextMinRunes:
		return nil, fmt.Errorf("%w: text is too short to be a CV; pass the person's CV verbatim, or send them to %s to add one",
			assistant.ErrInvalidArguments, profileURL)
	case n > cvTextMaxRunes:
		return nil, fmt.Errorf("%w: text is longer than %d characters", assistant.ErrInvalidArguments, cvTextMaxRunes)
	}
	if h.resume == nil || !h.resume.resume.Enabled() {
		return nil, unavailable("CV storage")
	}
	meta, err := h.resume.storeFirstCV(ctx, userID, resumeUpload{
		Data: []byte(text), ContentType: "text/plain; charset=utf-8", Text: text,
	})
	if errors.Is(err, resume.ErrAlreadyStored) {
		return nil, fiber.NewError(fiber.StatusConflict, "this account already has a CV. "+
			"A CV is replaced only on the website, at "+profileURL+", so the document the person chose "+
			"is never overwritten from a chat")
	}
	if err != nil {
		return nil, err
	}
	return addCVResult{
		Stored: meta.Present, UploadedAt: meta.UploadedAt,
		Suggested: suggestedFromCV(text), Next: addCVNext,
	}, nil
}

// suggestedFromCV is what onboarding pre-fills from an upload: the categories the CV spans as
// specializations, its grade as the one seniority, its skills.
func suggestedFromCV(text string) suggestedProfile {
	prof := resumeProfile(text)
	out := suggestedProfile{Specializations: prof.Categories, Seniorities: []string{}, Skills: prof.Skills}
	if prof.Seniority != "" {
		out.Seniorities = []string{prof.Seniority}
	}
	return out
}
