package sources

import "testing"

func TestCrawlRegistryRequiresCompleteCredentialPairs(t *testing.T) {
	providers := []struct {
		name      string
		idEnv     string
		secretEnv string
	}{
		{"adzuna", "ADZUNA_APP_ID", "ADZUNA_APP_KEY"},
		{"careerjet", "CAREERJET_API_KEY", "CAREERJET_USER_IP"},
		{"francetravail", "FRANCE_TRAVAIL_CLIENT_ID", "FRANCE_TRAVAIL_CLIENT_SECRET"},
	}
	cases := []struct {
		name string
		id   string
		key  string
		want bool
	}{
		{name: "id_only", id: "test-id"},
		{name: "secret_only", key: "test-secret"},
		{name: "complete", id: "test-id", key: "test-secret", want: true},
	}

	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv(provider.idEnv, tc.id)
					t.Setenv(provider.secretEnv, tc.key)

					_, registered := All(NewClient())[provider.name]
					if registered != tc.want {
						t.Errorf("All(client) registered %q = %t, want %t", provider.name, registered, tc.want)
					}
				})
			}
		})
	}
}

func TestCrawlRegistryRequiresJoobleKey(t *testing.T) {
	for key, want := range map[string]bool{"": false, "test-key": true} {
		t.Setenv("JOOBLE_API_KEY", key)
		if _, registered := All(NewClient())["jooble"]; registered != want {
			t.Errorf("All(client) registered jooble with key %q = %t, want %t", key, registered, want)
		}
	}
}
