package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/matchanalysis"
)

// StreamMatchAnalysis runs the three-stage fit chain over Server-Sent Events, emitting stage
// progress, best-effort thinking tokens, and each section as it resolves, then caching
// the final analysis exactly as PostMatchAnalysis does. Cookie or API key; unknown slug 404.
// The stream always opens with a `meta` event (has_cv); when no CV is stored it closes
// after that. Everything the stream needs is captured before the body writer starts,
// because the fiber ctx is released once this handler returns.
func (a *API) StreamMatchAnalysis(c *fiber.Ctx) error {
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
	// Gate on points before opening the stream (the fiber ctx is still valid here, so an
	// out-of-credits new job returns a real 402 instead of an SSE error). Only a CV-backed
	// request would run the LLM; without one the stream just reports has_cv. A recompute is
	// free — only a new analysis is charged, and only after it persists (in the writer).
	isNew := false
	if hasCV {
		var err error
		if isNew, err = a.matchIsNew(c.Context(), userID, job.ID); err != nil {
			return err
		}
		if isNew {
			if bal := a.creditsBalance(c.Context(), userID); bal != nil && bal.Remaining < a.credits.Cost(credits.FeatureMatch) {
				return creditsError(c, *bal)
			}
		}
	}
	cvUploadedAt, _ := a.cvUploadedAt(c, userID)
	profile, _ := a.userProfile.Get(c.Context(), userID)

	input := matchanalysis.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         a.companyInfo(c, job.CompanySlug),
		CVText:              cvText,
		StructuredResume:    a.structuredResumeJSON(c, userID),
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

	// The server's 10s WriteTimeout would kill this long-lived stream mid-analysis, so
	// clear the connection's write deadline for the SSE response only (captured here while
	// the ctx is valid; used inside the writer, which runs after this handler returns).
	conn := c.Context().Conn()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		if conn != nil {
			_ = conn.SetWriteDeadline(time.Time{})
		}
		start := time.Now()
		log.Printf("matchanalysis: stream start user=%d job=%d has_cv=%v", userID, job.ID, hasCV)
		writeSSE(w, "meta", map[string]bool{"has_cv": hasCV})
		if !hasCV {
			return
		}
		// A background context: the fiber request ctx is gone by now, and each LLM call
		// is already bounded by the client's per-call timeout.
		ctx := context.Background()
		events := 0

		// A long stage (a silent LLM call with no thinking tokens) would let the
		// connection go quiet long enough for nginx's proxy_read_timeout to sever it
		// mid-analysis — the client sees a bare "Connection lost". A periodic SSE comment
		// keeps bytes flowing so the stream survives silent stages. The ticker goroutine
		// and the stage callback both write to w, so a mutex serializes them (bufio.Writer
		// is not safe for concurrent use).
		var mu sync.Mutex
		stopHeartbeat := make(chan struct{})
		var heartbeat sync.WaitGroup
		heartbeat.Add(1)
		go func() {
			defer heartbeat.Done()
			t := time.NewTicker(15 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-t.C:
					mu.Lock()
					writeComment(w, "keepalive")
					mu.Unlock()
				}
			}
		}()

		analysis, err := a.matchAnalysis.AnalyzeStream(ctx, input, func(e matchanalysis.Event) {
			events++
			mu.Lock()
			writeSSE(w, string(e.Kind), e)
			mu.Unlock()
		})
		close(stopHeartbeat)
		heartbeat.Wait()
		if err != nil {
			log.Printf("matchanalysis: stream FAILED user=%d job=%d dur=%s events=%d: %v", userID, job.ID, time.Since(start).Round(time.Millisecond), events, err)
			writeSSE(w, "stream_error", map[string]string{"message": "analysis failed"})
			return
		}
		if analysis == nil {
			writeSSE(w, "stream_error", map[string]string{"message": "analysis unavailable"})
			return
		}
		a.cacheAnalysis(ctx, userID, job, cvUploadedAt, analysis)
		if isNew {
			a.debitMatch(ctx, userID, job.ID)
		}
		log.Printf("matchanalysis: stream DONE user=%d job=%d dur=%s events=%d overall=%d", userID, job.ID, time.Since(start).Round(time.Millisecond), events, analysis.OverallScore)
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

// writeComment writes an SSE comment line — ignored by EventSource — as a heartbeat that
// keeps the connection producing bytes through long, silent stages. A write error (client
// gone) is swallowed, exactly like writeSSE.
func writeComment(w *bufio.Writer, text string) {
	if _, err := fmt.Fprintf(w, ": %s\n\n", text); err != nil {
		return
	}
	_ = w.Flush()
}
