package handler

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cache"
	"github.com/strelov1/freehire/internal/catalogstats"
)

func ogApp(c cache.Cache, est catalogstats.Estimator) *fiber.App {
	h := &ogHandlers{cache: c, estimator: est}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"))
	return app
}

func doGetImage(t *testing.T, app *fiber.App, target string) *http.Response {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func decodePNGResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	defer resp.Body.Close()
	if ct := resp.Header.Get(fiber.HeaderContentType); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(body)); err != nil {
		t.Errorf("response is not a valid PNG: %v", err)
	}
}

func TestOGOpen_ServesAPNGFromThePublishedSnapshot(t *testing.T) {
	c := cache.NewMemory()
	if err := catalogstats.Store(context.Background(), c, publishedSnapshot()); err != nil {
		t.Fatalf("Store: %v", err)
	}

	app := ogApp(c, stubEstimator{value: 9_999_999})
	resp := doGetImage(t, app, "/api/v1/og/open.png")

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get(fiber.HeaderCacheControl); cc != ogImageCacheControl {
		t.Errorf("Cache-Control = %q, want %q", cc, ogImageCacheControl)
	}
	decodePNGResponse(t, resp)
}

// The endpoint must answer before the first rollup run, and while the cache is
// down, with a card built from the estimate rather than an error — same
// guarantee CatalogScale gives its JSON callers.
func TestOGAbout_DegradesRatherThanFailing(t *testing.T) {
	app := ogApp(cache.NewMemory(), stubEstimator{value: 3_150_000})
	resp := doGetImage(t, app, "/api/v1/og/about.png")

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 on a cold cache", resp.StatusCode)
	}
	decodePNGResponse(t, resp)
}

func TestOGOpen_SurvivesEverythingFailing(t *testing.T) {
	app := ogApp(cache.NewMemory(), stubEstimator{err: errors.New("database down")})
	resp := doGetImage(t, app, "/api/v1/og/open.png")

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 even with no figure available", resp.StatusCode)
	}
	decodePNGResponse(t, resp)
}
