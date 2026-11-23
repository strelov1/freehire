package interviewscheduling

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

const composePrompt = `Write the complete scheduling email reply in JSON with one key, "body". Choose one of the complete allowed replies below and reproduce it verbatim. Preserve every time, link, and line break. Do not add personal detail, promises, attachments, addresses or other links.`

// compose lets the model choose the reply wording while the outbound text is still
// confined to vetted language and facts supplied by the slot system.
func (w *Worker) compose(ctx context.Context, row db.ClaimInterviewSchedulingThreadRow, kind, parentID, body string) string {
	if w.model == nil {
		return body
	}
	modelBody := body
	bookingURL := ""
	if kind == "offer" {
		if _, tail, ok := strings.Cut(body, w.origin+"/booking/"); ok {
			bookingURL = w.origin + "/booking/" + tail
			modelBody = strings.Replace(body, bookingURL, "[booking link]", 1)
		}
	}
	alternatives := []string{modelBody}
	if row.Language == "en" {
		alternate := strings.Replace(modelBody, "Hi,", "Hello,", 1)
		alternate = strings.Replace(alternate, "Thanks,", "Thank you,", 1)
		if alternate != modelBody {
			alternatives = append(alternatives, alternate)
		}
	}
	if _, err := w.plans.RequirePaid(ctx, row.UserID); err != nil {
		return body
	}
	ref := fmt.Sprintf("interview-wording:%d:%s:%s:%d", row.ID, parentID, kind, row.Followups)
	if _, err := w.plans.Consume(ctx, row.UserID, plan.FeatureMailReply, ref); err != nil {
		return body
	}
	client := llmkey.Bind(plan.WithPaidAI(ctx), w.keys, w.model, row.UserID, llm.Feature("interview-scheduling"))
	input, _ := json.Marshal(alternatives)
	response, err := client.GenerateJSON(ctx, composePrompt, string(input))
	if err == nil {
		var out struct{ Body string }
		if json.Unmarshal([]byte(response), &out) == nil {
			draft := strings.TrimSpace(out.Body)
			if slices.Contains(alternatives, draft) {
				return strings.Replace(draft, "[booking link]", bookingURL, 1)
			}
		}
	}
	_ = w.plans.Release(context.WithoutCancel(ctx), row.UserID, plan.FeatureMailReply, ref)
	return body
}
