package discordbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_EditOriginalResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{"success", http.StatusOK, false},
		{"non-2xx", http.StatusNotFound, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			var gotBody map[string]string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotBody)
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			c := NewClientWithBase("bot-token", srv.URL)
			err := c.EditOriginalResponse(context.Background(), "app123", "tok456", "hello")

			if (err != nil) != tt.wantErr {
				t.Fatalf("EditOriginalResponse() err = %v, wantErr %v", err, tt.wantErr)
			}
			if gotMethod != http.MethodPatch {
				t.Errorf("method = %q, want PATCH", gotMethod)
			}
			wantPath := "/webhooks/app123/tok456/messages/@original"
			if gotPath != wantPath {
				t.Errorf("path = %q, want %q", gotPath, wantPath)
			}
			if gotAuth != "" {
				t.Errorf("Authorization header = %q, want empty (interaction token authorizes this call)", gotAuth)
			}
			if gotBody["content"] != "hello" {
				t.Errorf("body content = %q, want %q", gotBody["content"], "hello")
			}
		})
	}
}

func TestClient_RegisterCommands(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{"success", http.StatusOK, false},
		{"non-2xx", http.StatusUnauthorized, true},
	}

	commands := []Command{
		{
			Name:        "link",
			Description: "Link your freehire account",
			Options: []CommandOption{
				{Type: CommandOptionTypeString, Name: "token", Description: "Link token", Required: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			var gotBody []Command

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotBody)
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			c := NewClientWithBase("bot-token", srv.URL)
			err := c.RegisterCommands(context.Background(), "app123", "guild456", commands)

			if (err != nil) != tt.wantErr {
				t.Fatalf("RegisterCommands() err = %v, wantErr %v", err, tt.wantErr)
			}
			if gotMethod != http.MethodPut {
				t.Errorf("method = %q, want PUT", gotMethod)
			}
			wantPath := "/applications/app123/guilds/guild456/commands"
			if gotPath != wantPath {
				t.Errorf("path = %q, want %q", gotPath, wantPath)
			}
			wantAuth := "Bot bot-token"
			if gotAuth != wantAuth {
				t.Errorf("Authorization header = %q, want %q", gotAuth, wantAuth)
			}
			if len(gotBody) != 1 || gotBody[0].Name != "link" {
				t.Errorf("body = %+v, want one command named %q", gotBody, "link")
			}
		})
	}
}
