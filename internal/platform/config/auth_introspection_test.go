package config

import (
	"strings"
	"testing"
)

func TestAuthIntrospectionSecretConfiguration(t *testing.T) {
	for _, secret := range []string{"", "short", strings.Repeat("s", 32)} {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("AUTH_INTROSPECTION_SECRET", " "+secret+" ")
			if got := Load().AuthIntrospectionSecret; got != secret {
				t.Fatalf("configured secret was not trimmed")
			}
			err := (Settings{JWTSecret: strings.Repeat("j", 32), AuthIntrospectionSecret: secret}).Validate()
			if (err != nil) != (secret == "short") {
				t.Fatalf("unexpected secret validation result: %v", err)
			}
		})
	}
}
