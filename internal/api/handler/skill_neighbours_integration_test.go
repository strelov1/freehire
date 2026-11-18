//go:build integration

// Integration test for the skill-neighbours read: it serves one skill's rows of the
// insights_skill_neighbours snapshot, most frequent first, and an unknown skill or an
// empty snapshot as an empty list rather than an error.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSkillNeighboursEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/skills/:slug/neighbours", statsContract(h))

	type skillNeighbour struct {
		Skill string `json:"skill"`
		Count int64  `json:"count"`
	}
	type envelope struct {
		Data []skillNeighbour `json:"data"`
		Meta struct {
			Skill string `json:"skill"`
		} `json:"meta"`
	}
	get := func(slug string) envelope {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/stats/skills/"+slug+"/neighbours", nil))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env
	}

	if env := get("go"); env.Data == nil || len(env.Data) != 0 {
		t.Fatalf("empty snapshot: data = %#v, want an empty list", env.Data)
	}

	for _, p := range []db.InsertSkillNeighbourParams{
		{Skill: "go", Neighbour: "sql", Count: 5},
		{Skill: "go", Neighbour: "docker", Count: 40},
		{Skill: "go", Neighbour: "kubernetes", Count: 12},
		{Skill: "jira", Neighbour: "confluence", Count: 99},
	} {
		if err := q.InsertSkillNeighbour(ctx, p); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// The slug is matched case-insensitively, the same as the glossary route.
	env := get("Go")
	want := []skillNeighbour{{"docker", 40}, {"kubernetes", 12}, {"sql", 5}}
	if !reflect.DeepEqual(env.Data, want) || env.Meta.Skill != "go" {
		t.Fatalf("got %+v (meta %q), want %+v for go only", env.Data, env.Meta.Skill, want)
	}
	if env := get("rust"); len(env.Data) != 0 {
		t.Fatalf("unknown skill: got %+v, want empty", env.Data)
	}
}
