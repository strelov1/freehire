package config

import (
	"testing"
	"time"
)

func TestLoad_LLMFromEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://gw.example/v1")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_MODEL", "some-model")

	s := Load()
	if s.LLMBaseURL != "https://gw.example/v1" || s.LLMAPIKey != "key-123" || s.LLMModel != "some-model" {
		t.Errorf("LLM settings = %q/%q/%q, want the env values", s.LLMBaseURL, s.LLMAPIKey, s.LLMModel)
	}
}

func TestLoad_LLMEmptyWhenUnset(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	s := Load()
	if s.LLMBaseURL != "" || s.LLMAPIKey != "" || s.LLMModel != "" {
		t.Errorf("LLM settings should be empty when unset, got %q/%q/%q", s.LLMBaseURL, s.LLMAPIKey, s.LLMModel)
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

func TestLoad_LangfuseFromEnv(t *testing.T) {
	t.Setenv("LANGFUSE_BASE_URL", "https://us.cloud.langfuse.com")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-1")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-1")

	s := Load()
	if s.LangfuseBaseURL != "https://us.cloud.langfuse.com" || s.LangfusePublicKey != "pk-1" || s.LangfuseSecretKey != "sk-1" {
		t.Errorf("Langfuse settings = %q/%q/%q, want the env values",
			s.LangfuseBaseURL, s.LangfusePublicKey, s.LangfuseSecretKey)
	}
}

func TestLoad_LangfuseEmptyWhenUnset(t *testing.T) {
	t.Setenv("LANGFUSE_BASE_URL", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "")
	t.Setenv("LANGFUSE_SECRET_KEY", "")

	s := Load()
	if s.LangfuseBaseURL != "" || s.LangfusePublicKey != "" || s.LangfuseSecretKey != "" {
		t.Errorf("Langfuse settings should be empty when unset, got %q/%q/%q",
			s.LangfuseBaseURL, s.LangfusePublicKey, s.LangfuseSecretKey)
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
