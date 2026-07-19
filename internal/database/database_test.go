package database

import "testing"

// poolConfig must cap MaxConns when the DSN is silent (so the worker fleet can't
// exhaust Postgres slots) but yield to an explicit pool_max_conns override.
func TestPoolConfig_MaxConns(t *testing.T) {
	t.Run("caps to default when DSN omits pool_max_conns", func(t *testing.T) {
		cfg, err := poolConfig("postgres://u:p@localhost:5432/db")
		if err != nil {
			t.Fatalf("poolConfig: %v", err)
		}
		if cfg.MaxConns != defaultMaxConns {
			t.Errorf("MaxConns = %d, want %d", cfg.MaxConns, defaultMaxConns)
		}
	})

	t.Run("respects an explicit pool_max_conns", func(t *testing.T) {
		cfg, err := poolConfig("postgres://u:p@localhost:5432/db?pool_max_conns=30")
		if err != nil {
			t.Fatalf("poolConfig: %v", err)
		}
		if cfg.MaxConns != 30 {
			t.Errorf("MaxConns = %d, want 30", cfg.MaxConns)
		}
	})
}
