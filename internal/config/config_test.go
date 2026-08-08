package config

import (
	"strings"
	"testing"
)

func TestSettings_Validate(t *testing.T) {
	cases := []struct {
		name     string
		settings Settings
		wantErr  string
	}{
		{
			name: "valid dev config",
			settings: Settings{
				Env:            "development",
				CookieSecure:   false,
				FrontendOrigin: "http://localhost:5173",
				JWTSecret:      "short",
			},
			wantErr: "",
		},
		{
			name: "valid production config",
			settings: Settings{
				Env:            "production",
				CookieSecure:   true,
				FrontendOrigin: "https://freehire.me",
				JWTSecret:      "12345678901234567890123456789012",
			},
			wantErr: "",
		},
		{
			name: "production with insecure cookie",
			settings: Settings{
				Env:            "production",
				CookieSecure:   false,
				FrontendOrigin: "https://freehire.me",
				JWTSecret:      "12345678901234567890123456789012",
			},
			wantErr: "COOKIE_SECURE must be true in production",
		},
		{
			name: "HTTPS origin with insecure cookie",
			settings: Settings{
				Env:            "development",
				CookieSecure:   false,
				FrontendOrigin: "https://freehire.me",
				JWTSecret:      "12345678901234567890123456789012",
			},
			wantErr: "COOKIE_SECURE must be true when using HTTPS origin",
		},
		{
			name: "production with short JWT secret",
			settings: Settings{
				Env:            "production",
				CookieSecure:   true,
				FrontendOrigin: "https://freehire.me",
				JWTSecret:      "too-short-secret",
			},
			wantErr: "JWT_SECRET must be at least 32 characters in production",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.settings.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %v, want substring %q", err, tc.wantErr)
				}
			}
		})
	}
}
