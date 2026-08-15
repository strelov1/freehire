// Production entry point: runs the adapter-node server across several processes.
//
// `build/index.js` is a single Node process, and Node is single-threaded, so the
// SSR tier used one of host-2's sixteen cores however busy the box got. Measured
// on the idle blue/green colour with `perf/k6` at PROFILE=saturation: one process
// holds ~130 req/s, and past that p95 goes from half a second to twenty-five;
// four processes hold ~250. The 2026-08-14 peak was 204 req/s, which is why the
// accept queue kept filling and the watchdog kept restarting the process.
//
// cluster forks workers that share one listening socket — the primary accepts and
// hands connections out — so the port, the systemd unit and the blue/green layout
// are unchanged. Each worker is a full copy of the server (~370 MB resident), and
// that memory comes out of the page cache Meilisearch depends on, which is why the
// default is four rather than one-per-core: eight measured WORSE at 200 req/s
// (0.87 s vs 0.50 s p95) because the extra workers compete with ingest, Meili and
// Postgres for the same sixteen cores.
import cluster from 'node:cluster';

const workers = Number(process.env.WEB_CLUSTER_WORKERS || 4);

if (workers <= 1) {
  // One worker would add a supervisor process for nothing. Load the server here
  // so WEB_CLUSTER_WORKERS=1 is a genuine escape hatch back to the old shape.
  await import('./build/index.js');
} else if (cluster.isPrimary) {
  console.log(`cluster primary ${process.pid}: forking ${workers} workers`);
  for (let i = 0; i < workers; i++) cluster.fork();

  // A worker that dies takes its in-flight requests with it. Replacing it keeps
  // capacity flat instead of silently decaying towards a single process — which
  // would look exactly like the problem this file exists to fix.
  cluster.on('exit', (worker, code, signal) => {
    console.log(`worker ${worker.process.pid} exited (${signal || code}) — forking a replacement`);
    cluster.fork();
  });
} else {
  await import('./build/index.js');
}
