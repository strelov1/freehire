package handler

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/recentfeed"
)

func TestPublicStreamsCloseOnServerShutdown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mount func(*fiber.App)
	}{
		{
			name: "recent feed",
			mount: func(app *fiber.App) {
				b := recentfeed.NewBroadcaster(1)
				b.Publish(recentfeed.Entry{Kind: recentfeed.KindSingle, Title: "Engineer"})
				h := newRecentFeedHandlers(b, nil)
				h.pingInterval = time.Hour
				app.Get("/stream", h.StreamRecentJobs)
			},
		},
		{
			name: "embedding progress",
			mount: func(app *fiber.App) {
				h := newEmbedProgressHandlers(&fakeProgress{current: queuedProgress(10)}, nil)
				h.pingInterval = time.Hour
				app.Get("/stream", h.Stream)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			tc.mount(app)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			serveErr := make(chan error, 1)
			go func() { serveErr <- app.Listener(ln) }()
			t.Cleanup(func() { _ = app.ShutdownWithTimeout(time.Second) })

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ln.Addr().String()+"/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			reader := bufio.NewReader(resp.Body)
			readSSEFrame(t, reader) // The stream is established, then idle until shutdown.

			if err := app.ShutdownWithTimeout(time.Second); err != nil {
				t.Fatalf("shutdown with a connected stream: %v", err)
			}
			if body, err := io.ReadAll(reader); err != nil || len(body) != 0 {
				t.Fatalf("stream did not end cleanly: body=%q, err=%v", body, err)
			}
			if err := <-serveErr; err != nil {
				t.Fatalf("listener: %v", err)
			}
		})
	}
}
