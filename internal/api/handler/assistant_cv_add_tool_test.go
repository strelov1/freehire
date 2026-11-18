package handler

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/blobstore"
)

// A CV long enough to pass the floor, naming a title the dictionaries resolve and skills
// they tag.
const addCVText = "Jane Doe\nSenior Backend Engineer\n\n" +
	"Experience: Acme Corp, 2019-2024. Built payment services in Golang and PostgreSQL, " +
	"ran them on Kubernetes and cut p99 latency from 900ms to 120ms."

func addCVHandlers(blobs blobstore.Store, repo *fakeResumeRepo) *assistantHandlers {
	return &assistantHandlers{resume: &resumeHandlers{resume: resume.New(blobs, repo)}}
}

func runAddCV(t *testing.T, h *assistantHandlers, text string) (addCVResult, error) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"text": text})
	out, err := h.addCVTool(testMCPOrigin).Run(context.Background(), 1, raw)
	if err != nil {
		return addCVResult{}, err
	}
	res, ok := out.(addCVResult)
	if !ok {
		t.Fatalf("result type %T, want addCVResult", out)
	}
	return res, nil
}

func fiberStatus(err error) int {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return 0
}

// An account with no CV gets its text stored as the CV, through the website's own upload use
// case, and is handed back what it suggests for the profile — in update_profile's field names.
func TestAddCV_StoresTheTextAndSuggestsProfileValues(t *testing.T) {
	blobs, repo := newFakeResumeBlobs(), &fakeResumeRepo{}
	res, err := runAddCV(t, addCVHandlers(blobs, repo), addCVText)
	if err != nil {
		t.Fatalf("add_cv: %v", err)
	}
	if got := string(blobs.objs[repo.key]); got != addCVText || len(blobs.objs) != 1 {
		t.Fatalf("stored CV = %q, want the text verbatim", got)
	}
	if !res.Stored || res.UploadedAt == nil {
		t.Errorf("result = %+v, want stored with an upload time", res)
	}
	if !slices.Contains(res.Suggested.Skills, "go") || !slices.Contains(res.Suggested.Skills, "postgresql") {
		t.Errorf("suggested skills = %v, want go and postgresql", res.Suggested.Skills)
	}
	if !slices.Equal(res.Suggested.Seniorities, []string{"senior"}) {
		t.Errorf("suggested seniorities = %v, want [senior]", res.Suggested.Seniorities)
	}
	if len(res.Suggested.Specializations) == 0 {
		t.Errorf("suggested specializations empty, want the backend category")
	}
	if !strings.Contains(res.Next, "update_profile") || !strings.Contains(res.Next, "added to the profile's skills") {
		t.Errorf("next = %q, want it to say confirmed values are saved and found skills are added", res.Next)
	}
}

// The tool is for a first setup only: a stored CV is never replaced from a chat.
func TestAddCV_RefusesWhenTheAccountHasACV(t *testing.T) {
	blobs := newFakeResumeBlobs()
	blobs.objs["resumes/1"] = []byte("the PDF they chose")
	repo := &fakeResumeRepo{key: "resumes/1", set: true}
	_, err := runAddCV(t, addCVHandlers(blobs, repo), addCVText)
	if fiberStatus(err) != fiber.StatusConflict {
		t.Fatalf("err = %v, want a 409", err)
	}
	if !strings.Contains(err.Error(), testMCPOrigin+"/my/profile") {
		t.Errorf("refusal %q does not name where a CV is replaced", err)
	}
	if got := string(blobs.objs["resumes/1"]); got != "the PDF they chose" || len(blobs.objs) != 1 || repo.key != "resumes/1" {
		t.Errorf("stored CV changed to %q", got)
	}
}

func TestAddCV_RefusesTextOutsideTheBounds(t *testing.T) {
	for name, text := range map[string]string{
		"blank":     "   \n ",
		"too short": "Go developer, five years.",
		"too long":  strings.Repeat("a", cvTextMaxRunes+1),
	} {
		t.Run(name, func(t *testing.T) {
			blobs := newFakeResumeBlobs()
			_, err := runAddCV(t, addCVHandlers(blobs, &fakeResumeRepo{}), text)
			if !errors.Is(err, assistant.ErrInvalidArguments) && fiberStatus(err) != fiber.StatusBadRequest {
				t.Fatalf("err = %v, want a bad-argument refusal", err)
			}
			if len(blobs.objs) != 0 {
				t.Error("text outside the bounds was stored")
			}
		})
	}
}

func TestAddCV_StorageDisabled(t *testing.T) {
	_, err := runAddCV(t, addCVHandlers(nil, &fakeResumeRepo{}), addCVText)
	if fiberStatus(err) != fiber.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503", err)
	}
}

// The tool takes the person's own CV and says so; it never invites the agent to write one, and
// it is honest that found skills reach the profile.
func TestAddCV_DescriptionTakesOnlyThePersonsCV(t *testing.T) {
	tool := (&assistantHandlers{}).addCVTool(testMCPOrigin)
	for _, banned := range []string{"told you", "dictat", "changes no profile"} {
		if strings.Contains(tool.Description, banned) {
			t.Errorf("description contains %q: %s", banned, tool.Description)
		}
	}
	for _, want := range []string{"verbatim", "Never compose", "added to the profile's skills"} {
		if !strings.Contains(tool.Description, want) {
			t.Errorf("description lacks %q: %s", want, tool.Description)
		}
	}
	text := tool.Schema["properties"].(map[string]any)["text"].(map[string]any)
	if text["minLength"] != cvTextMinRunes || text["maxLength"] != cvTextMaxRunes {
		t.Errorf("text schema = %v, want minLength %d and maxLength %d", text, cvTextMinRunes, cvTextMaxRunes)
	}
}

// Directory policy: a description says what the tool does, never which tool to call.
func TestSetupTools_DescriptionsNameNoOtherTool(t *testing.T) {
	h := &assistantHandlers{}
	tools := h.cvSetupMCPTools(testMCPOrigin)
	all := mcpAccountTools(h, testMCPOrigin)
	for _, tool := range tools {
		raw, _ := json.Marshal(struct {
			D string
			S any
		}{tool.Description, tool.Schema})
		for _, other := range all {
			if other.Name != tool.Name && strings.Contains(string(raw), other.Name) {
				t.Errorf("%s's description or schema names %s", tool.Name, other.Name)
			}
		}
	}
}

// Both setup tools are an OAuth grant's: storing a CV and reading its status are cookie-only
// over REST, so a full-scope key does not get them.
func TestSetupTools_GrantOnly(t *testing.T) {
	h := &assistantHandlers{}
	names := func(ts []assistant.Tool) []string {
		var out []string
		for _, t := range ts {
			out = append(out, t.Name)
		}
		return out
	}
	granted, keyed := names(mcpAccountTools(h, testMCPOrigin)), names(mcpKeyTools(h, testMCPOrigin))
	for _, name := range []string{"add_cv", "get_setup_status"} {
		if !slices.Contains(granted, name) {
			t.Errorf("%s not served to an OAuth grant", name)
		}
		if slices.Contains(keyed, name) {
			t.Errorf("%s served to an API key", name)
		}
	}
}
