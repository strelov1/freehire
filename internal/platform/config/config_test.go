package config

import (
	"bytes"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoad_LLMFromEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://gw.example/v1")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_MODEL", "some-model")

	s := Load()
	if s.BaseURL != "https://gw.example/v1" || s.APIKey != "key-123" || s.Model != "some-model" {
		t.Errorf("LLM settings = %q/%q/%q, want the env values", s.BaseURL, s.APIKey, s.Model)
	}
}

func TestLoad_GatewayAdminFromEnv(t *testing.T) {
	t.Setenv("LLM_ADMIN_URL", "https://gw.example")
	t.Setenv("LLM_ADMIN_USERNAME", "admin")
	t.Setenv("LLM_ADMIN_PASSWORD", "admin-123")
	t.Setenv("LLM_ADMIN_TEMPLATE_KEY", "vk-freehire-service")

	s := Load()
	if s.LLMAdminURL != "https://gw.example" || s.LLMAdminUsername != "admin" || s.LLMAdminPassword != "admin-123" {
		t.Errorf("admin settings = %q/%q/%q, want the env values",
			s.LLMAdminURL, s.LLMAdminUsername, s.LLMAdminPassword)
	}
	// The template key is what a minted credential copies its provider policy from.
	// Losing it does not disable attribution loudly — it mints keys the gateway then
	// refuses every provider — so it is read here rather than left to a nil check.
	if s.LLMAdminTemplateKey != "vk-freehire-service" {
		t.Errorf("template key = %q, want the env value", s.LLMAdminTemplateKey)
	}
}

// The administrative endpoint is NOT the inference one: the gateway serves chat under
// /v1 and administration under /api. Defaulting one from the other would encode a guess
// about how an operator wrote a URL, and would foreclose keeping the admin API off the
// public host entirely.
func TestLoad_GatewayAdminIsNotDerivedFromTheInferenceURL(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://gw.example/v1")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_ADMIN_URL", "")
	t.Setenv("LLM_ADMIN_USERNAME", "")
	t.Setenv("LLM_ADMIN_PASSWORD", "")

	s := Load()
	if s.LLMAdminURL != "" || s.LLMAdminUsername != "" || s.LLMAdminPassword != "" {
		t.Errorf("admin settings = %q/%q/%q, want all empty — an unset admin API disables attribution",
			s.LLMAdminURL, s.LLMAdminUsername, s.LLMAdminPassword)
	}
}

// A ceiling is policy this change deliberately ships without. Zero is what the gateway
// client reads as "send no limit at all", so the default must be zero rather than some
// number nobody chose.
func TestLoad_NoPerUserCeilingByDefault(t *testing.T) {
	t.Setenv("LLM_USER_MAX_BUDGET", "")
	t.Setenv("LLM_USER_RPM_LIMIT", "")

	s := Load()
	if s.LLMUserMaxBudget != 0 || s.LLMUserRPMLimit != 0 {
		t.Errorf("ceiling = %v/%v, want no ceiling until one is chosen", s.LLMUserMaxBudget, s.LLMUserRPMLimit)
	}
	if s.LLMUserBudgetWindow != "30d" {
		t.Errorf("budget window = %q, want the 30d default — it only matters once a budget exists",
			s.LLMUserBudgetWindow)
	}
}

func TestLoad_PerUserCeilingFromEnv(t *testing.T) {
	t.Setenv("LLM_USER_MAX_BUDGET", "2.5")
	t.Setenv("LLM_USER_RPM_LIMIT", "60")
	t.Setenv("LLM_USER_BUDGET_WINDOW", "7d")

	s := Load()
	if s.LLMUserMaxBudget != 2.5 || s.LLMUserRPMLimit != 60 || s.LLMUserBudgetWindow != "7d" {
		t.Errorf("ceiling = %v/%v/%q, want the env values",
			s.LLMUserMaxBudget, s.LLMUserRPMLimit, s.LLMUserBudgetWindow)
	}
}

// An unparseable ceiling must read as "no ceiling", never as "nothing is allowed": a typo
// in an environment variable should not refuse every call the deployment makes.
func TestLoad_AnUnparseableCeilingIsNoCeiling(t *testing.T) {
	t.Setenv("LLM_USER_MAX_BUDGET", "two dollars")

	if s := Load(); s.LLMUserMaxBudget != 0 {
		t.Errorf("max budget = %v, want 0 so a typo cannot refuse every call", s.LLMUserMaxBudget)
	}
}

func TestLoad_BulletTruncationEscapeHatchDefaultsOff(t *testing.T) {
	t.Setenv("CV_EDIT_ALLOW_BULLET_TRUNCATION", "")
	if s := Load(); s.CVEditAllowBulletTruncation {
		t.Fatal("CVEditAllowBulletTruncation defaults on — an unset env must keep the refuse")
	}
}

func TestLoad_BulletTruncationEscapeHatchFromEnv(t *testing.T) {
	t.Setenv("CV_EDIT_ALLOW_BULLET_TRUNCATION", "true")
	if s := Load(); !s.CVEditAllowBulletTruncation {
		t.Fatal("CVEditAllowBulletTruncation = false, want true from env")
	}
}

func TestLoad_MaxBulletsDefaultsToTwenty(t *testing.T) {
	t.Setenv("CV_MAX_BULLETS", "")
	if s := Load(); s.CVMaxBullets != 20 {
		t.Fatalf("CVMaxBullets = %d, want 20", s.CVMaxBullets)
	}
}

func TestLoad_AssistantMaxPromptDefaultsTo8000(t *testing.T) {
	t.Setenv("ASSISTANT_MAX_PROMPT", "")
	if s := Load(); s.AssistantMaxPrompt != 8000 {
		t.Fatalf("AssistantMaxPrompt = %d, want 8000", s.AssistantMaxPrompt)
	}
}

func TestLoad_AssistantMaxPromptFromEnv(t *testing.T) {
	t.Setenv("ASSISTANT_MAX_PROMPT", "15000")
	if s := Load(); s.AssistantMaxPrompt != 15000 {
		t.Fatalf("AssistantMaxPrompt = %d, want 15000", s.AssistantMaxPrompt)
	}
}

func TestLoad_AssistantMaxPromptRejectsNonPositive(t *testing.T) {
	t.Setenv("ASSISTANT_MAX_PROMPT", "0")
	if s := Load(); s.AssistantMaxPrompt != 8000 {
		t.Fatalf("AssistantMaxPrompt = %d after 0, want 8000", s.AssistantMaxPrompt)
	}
}

func TestLoad_MaxBulletsFromEnv(t *testing.T) {
	t.Setenv("CV_MAX_BULLETS", "50")
	if s := Load(); s.CVMaxBullets != 50 {
		t.Fatalf("CVMaxBullets = %d, want 50", s.CVMaxBullets)
	}
}

func TestLoad_MatchAnalysisDefaults(t *testing.T) {
	for _, key := range []string{
		"MATCH_ANALYSIS_MAX_COMMENT_RUNES",
		"MATCH_ANALYSIS_MAX_LIST_ITEM_RUNES",
		"MATCH_ANALYSIS_MAX_RECOMMEND_RUNES",
		"MATCH_ANALYSIS_MAX_REQ_TEXT_RUNES",
		"MATCH_ANALYSIS_MAX_REQ_EVIDENCE_RUNES",
		"MATCH_ANALYSIS_MAX_STRENGTHS",
		"MATCH_ANALYSIS_MAX_GAPS",
		"MATCH_ANALYSIS_MAX_REQUIREMENTS",
		"MATCH_ANALYSIS_MAX_SIGNALS",
		"MATCH_ANALYSIS_MAX_SIGNAL_QUOTE_RUNES",
		"MATCH_ANALYSIS_MAX_SIGNAL_INSIGHT_RUNES",
	} {
		t.Setenv(key, "")
	}
	s := Load()
	want := MatchAnalysisSettings{
		MaxCommentRunes: 240, MaxListItemRunes: 200, MaxRecommendRunes: 1200,
		MaxReqTextRunes: 200, MaxReqEvidenceRunes: 240,
		MaxStrengths: 6, MaxGaps: 6, MaxRequirements: 30,
		MaxSignals: 5, MaxSignalQuoteRunes: 200, MaxSignalInsightRunes: 200,
	}
	if s.MatchAnalysis != want {
		t.Fatalf("MatchAnalysis = %+v, want %+v", s.MatchAnalysis, want)
	}
}

func TestLoad_MatchAnalysisFromEnv(t *testing.T) {
	t.Setenv("MATCH_ANALYSIS_MAX_RECOMMEND_RUNES", "800")
	t.Setenv("MATCH_ANALYSIS_MAX_SIGNALS", "9")
	s := Load()
	if s.MatchAnalysis.MaxRecommendRunes != 800 {
		t.Fatalf("MaxRecommendRunes = %d, want 800", s.MatchAnalysis.MaxRecommendRunes)
	}
	if s.MatchAnalysis.MaxSignals != 9 {
		t.Fatalf("MaxSignals = %d, want 9", s.MatchAnalysis.MaxSignals)
	}
	if s.MatchAnalysis.MaxCommentRunes != 240 {
		t.Fatalf("MaxCommentRunes = %d, want untouched default 240", s.MatchAnalysis.MaxCommentRunes)
	}
}

func TestLoad_STTModelHasNoDefault(t *testing.T) {
	t.Setenv("STT_MODEL", "")

	// No default on purpose. Transcription is billed per minute of audio against an
	// assistant that is not metered, so a deployment that never named a model must not
	// find itself paying for one — it gets no microphone instead.
	if s := Load(); s.STTModel != "" {
		t.Errorf("STTModel = %q, want empty when unset", s.STTModel)
	}
}

func TestLoad_STTModelFromEnv(t *testing.T) {
	t.Setenv("STT_MODEL", "gpt-4o-transcribe")

	if s := Load(); s.STTModel != "gpt-4o-transcribe" {
		t.Errorf("STTModel = %q, want the env value", s.STTModel)
	}
}

func TestLoad_RealtimeModelHasNoDefault(t *testing.T) {
	t.Setenv("REALTIME_MODEL", "")

	// Same posture as STTModel above: Realtime audio is billed per minute, so a
	// deployment that never named a model must not find itself paying for one — it
	// gets no voice mode instead.
	if s := Load(); s.RealtimeModel != "" {
		t.Errorf("RealtimeModel = %q, want empty when unset", s.RealtimeModel)
	}
}

func TestLoad_RealtimeModelFromEnv(t *testing.T) {
	t.Setenv("REALTIME_MODEL", "gpt-realtime-2.1")

	if s := Load(); s.RealtimeModel != "gpt-realtime-2.1" {
		t.Errorf("RealtimeModel = %q, want the env value", s.RealtimeModel)
	}
}

func TestLoad_PIIFilterURLFromEnv(t *testing.T) {
	t.Setenv("PII_FILTER_URL", "http://127.0.0.1:8099/detect")

	if s := Load(); s.PIIFilterURL != "http://127.0.0.1:8099/detect" {
		t.Errorf("PIIFilterURL = %q, want the env value", s.PIIFilterURL)
	}
}

func TestLoad_PIIFilterURLEmptyWhenUnset(t *testing.T) {
	t.Setenv("PII_FILTER_URL", "")

	if s := Load(); s.PIIFilterURL != "" {
		t.Errorf("PIIFilterURL should be empty when unset, got %q", s.PIIFilterURL)
	}
}

func TestLoad_LLMEmptyWhenUnset(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	s := Load()
	if s.BaseURL != "" || s.APIKey != "" || s.Model != "" {
		t.Errorf("LLM settings should be empty when unset, got %q/%q/%q", s.BaseURL, s.APIKey, s.Model)
	}
}

func TestLoad_S3FromEnv(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "https://hel1.your-objectstorage.com")
	t.Setenv("S3_BUCKET", "freehire-resumes")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")

	s := Load()
	if s.S3Endpoint != "https://hel1.your-objectstorage.com" || s.S3Bucket != "freehire-resumes" ||
		s.S3AccessKey != "ak" || s.S3SecretKey != "sk" {
		t.Errorf("S3 settings = %q/%q/%q/%q, want the env values",
			s.S3Endpoint, s.S3Bucket, s.S3AccessKey, s.S3SecretKey)
	}
}

func TestLoad_S3EmptyWhenUnset(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")

	s := Load()
	if s.S3Endpoint != "" || s.S3Bucket != "" || s.S3AccessKey != "" || s.S3SecretKey != "" {
		t.Errorf("S3 settings should be empty when unset, got %q/%q/%q/%q",
			s.S3Endpoint, s.S3Bucket, s.S3AccessKey, s.S3SecretKey)
	}
}

func TestLoad_SentryFromEnv(t *testing.T) {
	t.Setenv("SENTRY_DSN", "https://pub@o1.ingest.sentry.io/42")
	t.Setenv("SENTRY_ENVIRONMENT", "production")

	s := Load()
	if s.SentryDSN != "https://pub@o1.ingest.sentry.io/42" || s.SentryEnvironment != "production" {
		t.Errorf("Sentry settings = %q/%q, want the env values", s.SentryDSN, s.SentryEnvironment)
	}
}

func TestLoad_SentryDefaultsWhenUnset(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("SENTRY_ENVIRONMENT", "")

	s := Load()
	if s.SentryDSN != "" {
		t.Errorf("SentryDSN should be empty when unset, got %q", s.SentryDSN)
	}
	if s.SentryEnvironment != "development" {
		t.Errorf("SentryEnvironment should default to development when unset, got %q", s.SentryEnvironment)
	}
}

func TestLoad_JWTSecretFromEnv(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cret")

	if got := Load().JWTSecret; got != "s3cret" {
		t.Errorf("JWTSecret = %q, want %q", got, "s3cret")
	}
}

func TestLoad_JWTTTLDefaultsWhenUnset(t *testing.T) {
	t.Setenv("JWT_TTL", "")

	if got := Load().JWTTTL; got != 30*24*time.Hour {
		t.Errorf("JWTTTL = %v, want 30d", got)
	}
}

func TestLoad_JWTTTLParsesDuration(t *testing.T) {
	t.Setenv("JWT_TTL", "1h30m")

	if got := Load().JWTTTL; got != 90*time.Minute {
		t.Errorf("JWTTTL = %v, want 1h30m", got)
	}
}

func TestLoad_JWTTTLFallsBackOnGarbage(t *testing.T) {
	t.Setenv("JWT_TTL", "not-a-duration")

	if got := Load().JWTTTL; got != 30*24*time.Hour {
		t.Errorf("JWTTTL = %v, want 30d fallback", got)
	}
}

func TestLoad_MeiliURLDefaultsWhenUnset(t *testing.T) {
	t.Setenv("MEILI_URL", "")

	if got := Load().MeiliURL; got != "http://localhost:7700" {
		t.Errorf("MeiliURL = %q, want default", got)
	}
}

func TestLoad_MeiliURLFromEnv(t *testing.T) {
	t.Setenv("MEILI_URL", "http://meili:7700")

	if got := Load().MeiliURL; got != "http://meili:7700" {
		t.Errorf("MeiliURL = %q, want env value", got)
	}
}

func TestLoad_RedisURLDefaultsWhenUnset(t *testing.T) {
	t.Setenv("REDIS_URL", "")

	if got := Load().RedisURL; got != "redis://localhost:6379/0" {
		t.Errorf("RedisURL = %q, want default", got)
	}
}

func TestLoad_RedisURLFromEnv(t *testing.T) {
	t.Setenv("REDIS_URL", "redis://redis:6379/0")

	if got := Load().RedisURL; got != "redis://redis:6379/0" {
		t.Errorf("RedisURL = %q, want env value", got)
	}
}

func TestLoad_MeiliKeyFromEnv(t *testing.T) {
	t.Setenv("MEILI_MASTER_KEY", "master-key")

	if got := Load().MeiliKey; got != "master-key" {
		t.Errorf("MeiliKey = %q, want %q", got, "master-key")
	}
}

func TestLoad_OAuthCredentialsFromEnv(t *testing.T) {
	t.Setenv("OAUTH_GOOGLE_CLIENT_ID", "gid")
	t.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "gsecret")

	got := Load().OAuth["google"]
	if got.ClientID != "gid" || got.ClientSecret != "gsecret" {
		t.Errorf("OAuth[google] = %+v, want gid/gsecret", got)
	}
}

func TestLoad_OAuthAppleCredentialsFromEnv(t *testing.T) {
	// The private key is a multi-line PEM, which does not survive a systemd
	// EnvironmentFile reliably — like GMAIL_TOKEN_KEY, it is stored base64-encoded
	// and decoded on load.
	const pem = "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"
	t.Setenv("OAUTH_APPLE_CLIENT_ID", "me.freehire.web")
	t.Setenv("OAUTH_APPLE_TEAM_ID", "25U9HN34VM")
	t.Setenv("OAUTH_APPLE_KEY_ID", "ZC7298D2TR")
	t.Setenv("OAUTH_APPLE_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte(pem)))

	got := Load().OAuth["apple"]
	if got.ClientID != "me.freehire.web" || got.TeamID != "25U9HN34VM" ||
		got.KeyID != "ZC7298D2TR" || got.PrivateKey != pem {
		t.Errorf("OAuth[apple] = %+v, want client id/team id/key id/decoded private key", got)
	}
}

func TestLoad_OAuthApplePrivateKeyInvalidBase64IsEmpty(t *testing.T) {
	t.Setenv("OAUTH_APPLE_PRIVATE_KEY", "not valid base64!!")

	if got := Load().OAuth["apple"].PrivateKey; got != "" {
		t.Errorf("PrivateKey = %q, want empty for invalid base64", got)
	}
}

func TestLoad_NativeAppleAndAuthV2(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	t.Setenv("AUTH_V2_ENABLED", "true")
	t.Setenv("MOBILE_AUTH_CALLBACKS", "web=https://freehire.me/my/reauth,ios=https://freehire.me/auth/callback")
	t.Setenv("APPLE_NATIVE_CLIENT_ID", "me.freehire.mobile")
	t.Setenv("APPLE_GRANT_ACTIVE_KEY_ID", "v2")
	t.Setenv("APPLE_GRANT_KEYS", "v1:"+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))+",v2:"+base64.StdEncoding.EncodeToString(key))
	s := Load()
	if !s.AuthV2Enabled || s.MobileAuthCallbacks["ios"] != "https://freehire.me/auth/callback" {
		t.Fatalf("v2 config not loaded: %+v", s.MobileAuthCallbacks)
	}
	if s.AppleNativeClientID != "me.freehire.mobile" || s.AppleGrantActiveKeyID != "v2" || !bytes.Equal(s.AppleGrantKeys["v2"], key) {
		t.Fatal("native Apple config not loaded")
	}
}

func TestSettingsValidateRejectsUnsafeProductionV2Callbacks(t *testing.T) {
	base := Settings{Env: "production", CookieSecure: true, JWTSecret: strings.Repeat("x", 32),
		FrontendOrigin: "https://freehire.me", AuthV2Enabled: true,
		MobileAuthCallbacks: map[string]string{"web": "https://freehire.me/my/reauth"}}
	for name, callback := range map[string]string{
		"unknown target": "https://freehire.me/callback",
		"insecure":       "http://freehire.me/callback",
		"userinfo":       "https://user@freehire.me/callback",
		"fragment":       "https://freehire.me/callback#token",
	} {
		t.Run(name, func(t *testing.T) {
			s := base
			s.MobileAuthCallbacks = map[string]string{"web": "https://freehire.me/my/reauth", "unknown": callback}
			if name != "unknown target" {
				s.MobileAuthCallbacks = map[string]string{"web": callback}
			}
			if err := s.Validate(); err == nil {
				t.Fatal("unsafe callback accepted")
			}
		})
	}
}

func TestSettingsValidateRequiresSameOriginWebReauthCallback(t *testing.T) {
	s := Settings{JWTSecret: strings.Repeat("x", 32), FrontendOrigin: "https://freehire.me", CookieSecure: true,
		AuthV2Enabled: true, MobileAuthCallbacks: map[string]string{"web": "https://auth.freehire.me/my/reauth"}}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "FRONTEND_ORIGIN") {
		t.Fatalf("Validate()=%v", err)
	}
}

func TestSettingsValidateRejectsPartialNativeApple(t *testing.T) {
	s := Settings{JWTSecret: strings.Repeat("x", 32), AppleNativeClientID: "me.freehire.mobile"}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "APPLE_GRANT") {
		t.Fatalf("Validate()=%v", err)
	}
}

func TestLoad_OAuthUnsetProviderIsZero(t *testing.T) {
	t.Setenv("OAUTH_LINKEDIN_CLIENT_ID", "")
	t.Setenv("OAUTH_LINKEDIN_CLIENT_SECRET", "")

	if got := Load().OAuth["linkedin"]; got != (OAuthCredentials{}) {
		t.Errorf("OAuth[linkedin] = %+v, want zero", got)
	}
}

func TestLoad_EmailNotifyFromEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-central-1")
	t.Setenv("NOTIFY_EMAIL_FROM", "notifications@freehire.me")

	s := Load()
	if s.AWSRegion != "eu-central-1" || s.NotifyEmailFrom != "notifications@freehire.me" {
		t.Errorf("email-notify settings = %q/%q, want the env values", s.AWSRegion, s.NotifyEmailFrom)
	}
}

func TestLoad_EmailNotifyEmptyWhenUnset(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("NOTIFY_EMAIL_FROM", "")

	s := Load()
	if s.AWSRegion != "" || s.NotifyEmailFrom != "" {
		t.Errorf("email-notify settings should be empty when unset, got %q/%q", s.AWSRegion, s.NotifyEmailFrom)
	}
}

func TestLoad_ExtensionRedirectAllowlistFromEnv(t *testing.T) {
	t.Setenv("EXTENSION_REDIRECT_ALLOWLIST", "abcdefghijklmnop, qrstuvwxyzabcdef")

	got := Load().ExtensionRedirectAllowlist
	want := []string{"abcdefghijklmnop", "qrstuvwxyzabcdef"}
	if !slices.Equal(got, want) {
		t.Errorf("ExtensionRedirectAllowlist = %v, want %v", got, want)
	}
}

func TestLoad_ExtensionRedirectAllowlistDropsBlankEntries(t *testing.T) {
	t.Setenv("EXTENSION_REDIRECT_ALLOWLIST", " , abcdefghijklmnop ,,  ")

	got := Load().ExtensionRedirectAllowlist
	want := []string{"abcdefghijklmnop"}
	if !slices.Equal(got, want) {
		t.Errorf("ExtensionRedirectAllowlist = %v, want %v", got, want)
	}
}

func TestLoad_ExtensionRedirectAllowlistNilWhenUnset(t *testing.T) {
	t.Setenv("EXTENSION_REDIRECT_ALLOWLIST", "")

	if got := Load().ExtensionRedirectAllowlist; got != nil {
		t.Errorf("ExtensionRedirectAllowlist should be nil when unset, got %v", got)
	}
}

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
				JWTSecret:      "12345678901234567890123456789012",
			},
			wantErr: "",
		},
		{
			name: "dev config with short JWT secret",
			settings: Settings{
				Env:            "development",
				CookieSecure:   false,
				FrontendOrigin: "http://localhost:5173",
				JWTSecret:      "short",
			},
			wantErr: "JWT_SECRET is required and must be at least 32 bytes",
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
			wantErr: "JWT_SECRET is required and must be at least 32 bytes",
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
