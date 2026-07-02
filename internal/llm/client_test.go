package llm

import "testing"

func TestSettings_Enabled(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
		want bool
	}{
		{"all set", Settings{BaseURL: "u", APIKey: "k", Model: "m"}, true},
		{"empty", Settings{}, false},
		{"missing model", Settings{BaseURL: "u", APIKey: "k"}, false},
		{"missing key", Settings{BaseURL: "u", Model: "m"}, false},
	}
	for _, c := range cases {
		if got := c.s.Enabled(); got != c.want {
			t.Errorf("%s: Enabled() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNewClient_UnconfiguredReturnsNilClientAndNoopFlush(t *testing.T) {
	client, flush, err := NewClient(Settings{}, "verdict")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client != nil {
		t.Error("client should be nil when the LLM is unconfigured")
	}
	if flush == nil {
		t.Fatal("flush must never be nil")
	}
	flush() // must be a safe no-op
}

func TestNewClient_LLMOnlyNoTracing(t *testing.T) {
	// openai.New does not dial on construction, so fake creds yield a usable client
	// without any network. No Langfuse ⇒ flush is a no-op.
	client, flush, err := NewClient(Settings{BaseURL: "https://gw.example/v1", APIKey: "k", Model: "m"}, "verdict")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("client should be non-nil when the LLM is configured")
	}
	flush()
}

func TestNewClient_WithTracing(t *testing.T) {
	client, flush, err := NewClient(Settings{
		BaseURL:           "https://gw.example/v1",
		APIKey:            "k",
		Model:             "m",
		LangfuseBaseURL:   "https://us.cloud.langfuse.com",
		LangfusePublicKey: "pk-lf-x",
		LangfuseSecretKey: "sk-lf-y",
	}, "verdict")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("client should be non-nil")
	}
	// flush drains an empty buffer: no generations queued, so no network call.
	flush()
}
