package oauth

import (
	"testing"
	"time"
)

func TestCodeStore_MintConsume(t *testing.T) {
	s := NewCodeStore(time.Minute)
	code, err := s.Mint(42)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if code == "" {
		t.Fatal("Mint returned an empty code")
	}
	userID, ok := s.Consume(code)
	if !ok || userID != 42 {
		t.Fatalf("Consume = (%d, %v), want (42, true)", userID, ok)
	}
}

func TestCodeStore_SingleUse(t *testing.T) {
	s := NewCodeStore(time.Minute)
	code, _ := s.Mint(7)
	if _, ok := s.Consume(code); !ok {
		t.Fatal("first Consume should succeed")
	}
	if _, ok := s.Consume(code); ok {
		t.Fatal("second Consume of the same code must fail")
	}
}

func TestCodeStore_Expires(t *testing.T) {
	s := NewCodeStore(time.Minute)
	now := time.Unix(1_000, 0)
	s.now = func() time.Time { return now }
	code, _ := s.Mint(9)

	now = now.Add(59 * time.Second) // still inside the TTL
	if _, ok := s.Consume(code); !ok {
		t.Fatal("code should still be valid before TTL")
	}

	code2, _ := s.Mint(9)
	now = now.Add(time.Minute + time.Second) // past the TTL
	if _, ok := s.Consume(code2); ok {
		t.Fatal("expired code must not be consumable")
	}
}

func TestCodeStore_UnknownCode(t *testing.T) {
	s := NewCodeStore(time.Minute)
	if _, ok := s.Consume("nope"); ok {
		t.Fatal("unknown code must not consume")
	}
	if _, ok := s.Consume(""); ok {
		t.Fatal("empty code must not consume")
	}
}

func TestCodeStore_CodesAreUnique(t *testing.T) {
	s := NewCodeStore(time.Minute)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		code, err := s.Mint(int64(i))
		if err != nil {
			t.Fatalf("Mint: %v", err)
		}
		if seen[code] {
			t.Fatalf("duplicate code minted: %q", code)
		}
		seen[code] = true
	}
}
