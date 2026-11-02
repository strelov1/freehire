package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProductionSourcesClientBlocksLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := NewClient().GetText(ctx, "https://127.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("sources client error = %v; want safehttp block for loopback target", err)
	}
}
