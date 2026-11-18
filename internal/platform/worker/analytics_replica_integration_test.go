//go:build integration

package worker

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Exercise the recovery checks and WAL fence against physical replication, including
// a connected standby that cannot catch up because replay is paused.
func TestAnalyticsPoolPhysicalReplicaFence(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	config := filepath.Join(directory, "postgresql.conf")
	hba := filepath.Join(directory, "pg_hba.conf")
	if err := os.WriteFile(config, []byte("listen_addresses='*'\nhba_file='/tmp/analytics-pg-hba.conf'\nshared_buffers='16MB'\nwal_level='replica'\nmax_wal_senders=5\nhot_standby=on\nrecovery_min_apply_delay='3s'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hba, []byte("local all all trust\nhost all all all scram-sha-256\nhost replication all all scram-sha-256\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(ctx) })
	hbaFile := testcontainers.ContainerFile{HostFilePath: hba, ContainerFilePath: "/tmp/analytics-pg-hba.conf", FileMode: 0o644}
	source, err := postgres.Run(ctx, "pgvector/pgvector:pg18", postgres.WithDatabase("hire"), postgres.WithUsername("hire"), postgres.WithPassword("hire"), postgres.WithConfigFile(config), postgres.BasicWaitStrategies(), testcontainers.WithFiles(hbaFile), network.WithNetwork([]string{"analytics-primary"}, nw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Terminate(ctx) })
	sourceURL, err := source.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	primary, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primary.Close)
	if _, err := primary.Exec(ctx, `CREATE TABLE analytics_probe(id int); INSERT INTO analytics_probe VALUES (1); CREATE ROLE stats_reader LOGIN PASSWORD 'hire'; GRANT SELECT ON analytics_probe TO stats_reader; GRANT EXECUTE ON FUNCTION pg_catalog.pg_is_wal_replay_paused() TO stats_reader;`); err != nil {
		t.Fatal(err)
	}
	replica, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "pgvector/pgvector:pg18", ExposedPorts: []string{"5432/tcp"},
			Networks:   []string{nw.Name},
			Files:      []testcontainers.ContainerFile{hbaFile, {HostFilePath: config, ContainerFilePath: "/tmp/analytics-postgresql.conf", FileMode: 0o644}},
			Cmd:        []string{"bash", "-ceu", `install -d -m 0700 -o postgres -g postgres /tmp/analytics-standby; export PGPASSWORD=hire; gosu postgres pg_basebackup -h analytics-primary -U hire -D /tmp/analytics-standby -X stream -R; exec gosu postgres postgres -D /tmp/analytics-standby -c config_file=/tmp/analytics-postgresql.conf`},
			WaitingFor: wait.ForLog("ready to accept read-only connections").WithStartupTimeout(90 * time.Second),
		}, Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replica.Terminate(ctx) })
	host, err := replica.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := replica.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	readerURL := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port.Port()), Path: "/hire", RawQuery: "sslmode=disable", User: url.UserPassword("stats_reader", "hire")}
	t.Setenv("ANALYTICS_DATABASE_URL", readerURL.String())
	adminURL := readerURL
	adminURL.User = url.UserPassword("hire", "hire")
	admin, err := pgxpool.New(ctx, adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := primary.Exec(ctx, `INSERT INTO analytics_probe VALUES (2)`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM analytics_probe`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("fixture must start behind primary: replica rows=%d, want 1", before)
	}
	reader, err := AnalyticsPool(ctx, primary)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Close)
	var count int
	if err := reader.QueryRow(ctx, `SELECT count(*) FROM analytics_probe`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("replica rows=%d, want 2 after WAL fence", count)
	}
	if _, err := admin.Exec(ctx, `SELECT pg_wal_replay_pause()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, `SELECT pg_wal_replay_resume()`) })
	if _, err := primary.Exec(ctx, `INSERT INTO analytics_probe VALUES (3)`); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	blocked, err := AnalyticsPool(deadline, primary)
	if blocked != nil {
		blocked.Close()
		t.Fatal("paused replica must not return a reader or primary fallback")
	}
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "analytics catch-up") {
		t.Fatalf("paused catch-up error=%v, want deadline", err)
	}
}
