package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
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

const addCVNext = "Nothing was saved to the profile. Show the person every suggested value, ask which " +
	"to keep, and save only the ones they confirm with update_profile. Their work history is read " +
	"in the background; get_setup_status reports when that is done."

// addCVTool stores CV text for an account that has none, through storeCV — the website's own
// upload use case, so the document, the free background extraction and the experience-bank
// import (cv_import: the text IS the stored CV each atom traces to) are the same as a paste on
// the profile page. See openspec/changes/mcp-cv-setup/design.md for why it refuses to replace.
func (h *assistantHandlers) addCVTool(origin string) assistant.Tool {
	profileURL := strings.TrimSuffix(origin, "/") + "/my/profile"
	return assistant.Tool{
		Name:   "add_cv",
		Title:  "Add my CV",
		Effect: assistant.EffectAdds,
		Description: "Give freehire the person's CV as text, for an account that has no CV yet. Send the " +
			"CV exactly as the person provided it — text they pasted, or the content of a file they " +
			"shared that you can read — or the work history, education and skills they told you, in " +
			"their words. Never invent, embellish or summarise: it is stored as their CV and its work " +
			"history is recorded as theirs. Between 100 and 30000 characters. Refused when the account " +
			"already has a CV; it is replaced on the website. Returns the profile values the CV " +
			"suggests (specializations, seniorities, skills) for the person to confirm; it changes no " +
			"profile setting.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The CV text, as the person gave it.",
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
		return nil, fmt.Errorf("%w: text is too short to be a CV; ask the person for their work history, education and skills", assistant.ErrInvalidArguments)
	case n > cvTextMaxRunes:
		return nil, fmt.Errorf("%w: text is longer than %d characters", assistant.ErrInvalidArguments, cvTextMaxRunes)
	}
	if h.resume == nil || !h.resume.resume.Enabled() {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "CV storage is not available")
	}
	current, err := h.resume.resume.Status(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current.Present {
		return nil, fiber.NewError(fiber.StatusConflict, "this account already has a CV. "+
			"A CV is replaced only on the website, at "+profileURL+", so the document the person chose "+
			"is never overwritten from a chat")
	}
	meta, err := h.resume.storeCV(ctx, userID, resumeUpload{
		Data: []byte(text), ContentType: "text/plain; charset=utf-8", Text: text,
	})
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
