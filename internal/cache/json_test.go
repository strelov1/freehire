package cache

import (
	"context"
	"testing"
	"time"
)

type payload struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func TestJSONRoundTrip(t *testing.T) {
	m, _ := newTestMemory(t)
	ctx := context.Background()
	want := payload{Name: "catalogue", Count: 3_300_658}

	if err := SetJSON(ctx, m, "k", want, time.Minute); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	got, found, err := GetJSON[payload](ctx, m, "k")
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !found {
		t.Fatal("GetJSON: found = false, want true for a key just set")
	}
	if got != want {
		t.Errorf("GetJSON = %+v, want %+v", got, want)
	}
}

func TestJSONMissReturnsZeroValue(t *testing.T) {
	m, _ := newTestMemory(t)

	got, found, err := GetJSON[payload](context.Background(), m, "absent")
	if err != nil {
		t.Fatalf("GetJSON on an absent key returned an error: %v", err)
	}
	if found {
		t.Error("GetJSON: found = true, want false")
	}
	if got != (payload{}) {
		t.Errorf("GetJSON = %+v, want the zero value on a miss", got)
	}
}

// A payload written by an older build may no longer decode into the current type. That
// must read as a miss so the caller recomputes, not as a hit carrying a half-filled
// value and not as a failure that wedges the caller until the key expires. The error
// still comes back, because "the cache holds something undecodable" is worth logging in
// a way an ordinary miss is not.
func TestJSONUndecodablePayloadIsAMiss(t *testing.T) {
	m, _ := newTestMemory(t)
	ctx := context.Background()

	if err := m.Set(ctx, "k", []byte(`{"count": "not a number"}`), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, found, err := GetJSON[payload](ctx, m, "k")
	if found {
		t.Error("GetJSON: found = true for an undecodable payload — the caller would use a half-filled value")
	}
	if got != (payload{}) {
		t.Errorf("GetJSON = %+v, want the zero value when decoding fails", got)
	}
	if err == nil {
		t.Error("GetJSON: err = nil for an undecodable payload — indistinguishable from an ordinary miss in logs")
	}
}

func TestJSONBackendErrorPropagates(t *testing.T) {
	c, mr := newTestRedisCache(t)
	mr.Close()

	_, found, err := GetJSON[payload](context.Background(), c, "k")
	if found {
		t.Error("GetJSON: found = true against a closed backend")
	}
	if err == nil {
		t.Error("GetJSON: err = nil against a closed backend")
	}
}
