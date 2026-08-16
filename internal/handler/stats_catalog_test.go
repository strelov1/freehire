package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cache"
	"github.com/strelov1/freehire/internal/catalogstats"
)

type stubEstimator struct {
	value int64
	err   error
}

func (s stubEstimator) EstimateOpenJobs(context.Context) (int64, error) { return s.value, s.err }

func catalogApp(c cache.Cache, est catalogstats.Estimator) *fiber.App {
	h := &statsHandlers{cache: c, estimator: est}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/stats/catalog", h.CatalogScale)
	return app
}

func publishedSnapshot() catalogstats.Snapshot {
	return catalogstats.Snapshot{
		OpenJobs:         3_300_658,
		Companies:        294_282,
		Sources:          227,
		ATSPlatforms:     93,
		TelegramChannels: 95,
		ComputedAt:       time.Unix(1_700_000_000, 0).UTC(),
	}
}

func decodeCatalog(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return data
}

func TestCatalogScale_ServesThePublishedSnapshot(t *testing.T) {
	c := cache.NewMemory()
	if err := catalogstats.Store(context.Background(), c, publishedSnapshot()); err != nil {
		t.Fatalf("Store: %v", err)
	}

	app := catalogApp(c, stubEstimator{value: 9_999_999})
	status, body := doGet(t, app, "/stats/catalog")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := decodeCatalog(t, body)

	for field, want := range map[string]float64{
		"open_jobs":         3_300_658,
		"companies":         294_282,
		"sources":           227,
		"ats_platforms":     93,
		"telegram_channels": 95,
	} {
		if got, _ := data[field].(float64); got != want {
			t.Errorf("%s = %v, want %v", field, data[field], want)
		}
	}
	if data["computed_at"] == nil {
		t.Error("computed_at is absent — a consumer cannot tell how stale the snapshot is")
	}
	if exact, _ := data["exact"].(bool); !exact {
		t.Error("exact = false for a published snapshot")
	}
}

// The endpoint must answer before the first worker run, and while Redis is down, with
// the approximate figure rather than an error — a transparency page that 500s is worse
// than one showing an estimate it labels as such.
func TestCatalogScale_DegradesRatherThanFailing(t *testing.T) {
	app := catalogApp(cache.NewMemory(), stubEstimator{value: 3_150_000})
	status, body := doGet(t, app, "/stats/catalog")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 on a cold cache", status)
	}
	data := decodeCatalog(t, body)

	if got, _ := data["open_jobs"].(float64); got != 3_150_000 {
		t.Errorf("open_jobs = %v, want the estimate 3150000", data["open_jobs"])
	}
	if exact, _ := data["exact"].(bool); exact {
		t.Error("exact = true for a degraded read — consumers cannot tell an estimate from a count")
	}
	// The registry figures cost nothing and need no cache, so they must survive.
	if got, _ := data["sources"].(float64); got <= 0 {
		t.Errorf("sources = %v on the degraded path, want the registry-derived count", data["sources"])
	}
}

func TestCatalogScale_SurvivesEverythingFailing(t *testing.T) {
	app := catalogApp(cache.NewMemory(), stubEstimator{err: errors.New("database down")})
	status, _ := doGet(t, app, "/stats/catalog")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 even with no figure available", status)
	}
}
