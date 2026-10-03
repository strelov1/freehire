package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func oauthMetadataApp() *fiber.App {
	h := &authHandlers{frontendOrigin: "https://freehire.me"}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/oauth/metadata/authorization-server", h.OAuthAuthorizationServerMetadata)
	app.Get("/api/v1/oauth/metadata/protected-resource", h.OAuthProtectedResourceMetadata)
	return app
}

func TestOAuthAuthorizationServerMetadata(t *testing.T) {
	app := oauthMetadataApp()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/oauth/metadata/authorization-server", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Issuer                        string   `json:"issuer"`
		AuthorizationEndpoint         string   `json:"authorization_endpoint"`
		TokenEndpoint                 string   `json:"token_endpoint"`
		RegistrationEndpoint          string   `json:"registration_endpoint"`
		CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AuthorizationEndpoint == "" || out.TokenEndpoint == "" || out.RegistrationEndpoint == "" {
		t.Errorf("missing an endpoint: %+v", out)
	}
	if len(out.CodeChallengeMethodsSupported) != 1 || out.CodeChallengeMethodsSupported[0] != "S256" {
		t.Errorf("code_challenge_methods_supported = %v, want [\"S256\"]", out.CodeChallengeMethodsSupported)
	}
}

func TestOAuthProtectedResourceMetadata(t *testing.T) {
	app := oauthMetadataApp()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/oauth/metadata/protected-resource", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Resource == "" || len(out.AuthorizationServers) == 0 {
		t.Errorf("out = %+v", out)
	}
}
