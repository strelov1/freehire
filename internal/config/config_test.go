package config

import (
	"slices"
	"testing"
	"time"
)

func TestLoad_LLMFromEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://gw.example/v1")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_MODEL", "some-model")

	s := Load()
	if s.LLM.BaseURL != "https://gw.example/v1" || s.LLM.APIKey != "key-123" || s.LLM.Model != "some-model" {
		t.Errorf("LLM settings = %q/%q/%q, want the env values", s.LLM.BaseURL, s.LLM.APIKey, s.LLM.Model)
	}
}

func TestLoad_GatewayAdminFromEnv(t *testing.T) {
	t.Setenv("LLM_ADMIN_URL", "https://gw.example")
	t.Setenv("LLM_ADMIN_KEY", "admin-123")

	s := Load()
	if s.LLMAdminURL != "https://gw.example" || s.LLMAdminKey != "admin-123" {
		t.Errorf("admin settings = %q/%q, want the env values", s.LLMAdminURL, s.LLMAdminKey)
	}
}

// The administrative endpoint is NOT the inference one: the gateway serves chat under
// /v1 and administration at its root. Defaulting one from the other would encode a guess
// about how an operator wrote a URL, and would foreclose keeping the admin API off the
// public host entirely.
func TestLoad_GatewayAdminIsNotDerivedFromTheInferenceURL(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://gw.example/v1")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_ADMIN_URL", "")
	t.Setenv("LLM_ADMIN_KEY", "")

	s := Load()
	if s.LLMAdminURL != "" || s.LLMAdminKey != "" {
		t.Errorf("admin settings = %q/%q, want both empty — an unset admin API disables attribution",
			s.LLMAdminURL, s.LLMAdminKey)
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
	if s.LLM.BaseURL != "" || s.LLM.APIKey != "" || s.LLM.Model != "" {
		t.Errorf("LLM settings should be empty when unset, got %q/%q/%q", s.LLM.BaseURL, s.LLM.APIKey, s.LLM.Model)
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
