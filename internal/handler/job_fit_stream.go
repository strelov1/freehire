package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/jobfit"
	"github.com/strelov1/freehire/internal/jobmatch"
)

// StreamJobFit runs the three-stage fit chain over Server-Sent Events, emitting stage
// progress, best-effort thinking tokens, and each section as it resolves, then caching
// the final analysis exactly as PostJobFit does. Cookie or API key; unknown slug 404.
// The stream always opens with a `meta` event (has_cv); when no CV is stored it closes
// after that. Everything the stream needs is captured before the body writer starts,
// because the fiber ctx is released once this handler returns.
func (a *API) StreamJobFit(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := a.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvText, hasCV, err := a.storedCVText(c, userID)
	if err != nil {
		return err
	}
	cvUploadedAt, _ := a.cvUploadedAt(c, userID)
	profile, _ := a.userProfile.Get(c.Context(), userID)

	input := jobfit.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         a.companyInfo(c, job.CompanySlug),
		CVText:              cvText,
		Match:               jobmatch.Compute(job.Skills, profile.Skills),
		JobWorkMode:         job.WorkMode,
		JobRemote:           job.Remote,
		JobLocation:         job.Location,
		JobRegions:          job.Regions,
		JobCountries:        job.Countries,
		LocationPreferences: string(profile.LocationPreferences),
	}

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no") // stop nginx buffering so events reach the browser promptly

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		writeSSE(w, "meta", map[string]bool{"has_cv": hasCV})
		if !hasCV {
			return
		}
		// A background context: the fiber request ctx is gone by now, and each LLM call
		// is already bounded by the client's per-call timeout.
		ctx := context.Background()
		analysis, err := a.jobFit.AnalyzeStream(ctx, input, func(e jobfit.Event) {
			writeSSE(w, string(e.Kind), e)
		})
		if err != nil {
			log.Printf("jobfit: stream analyze failed for user %d job %d: %v", userID, job.ID, err)
			writeSSE(w, "stream_error", map[string]string{"message": "analysis failed"})
			return
		}
		if analysis == nil {
			writeSSE(w, "stream_error", map[string]string{"message": "analysis unavailable"})
			return
		}
		a.cacheAnalysis(ctx, userID, job, cvUploadedAt, analysis)
	}))
	return nil
}

// writeSSE writes one named SSE event with a JSON data payload and flushes it. A write
// error (client disconnected) is swallowed — the stream is best-effort.
func writeSSE(w *bufio.Writer, event string, data any) {
	blob, err := json.Marshal(data)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, blob); err != nil {
		return
	}
	_ = w.Flush()
}
