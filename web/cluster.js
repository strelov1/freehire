// Production entry point: runs the adapter-node server across several processes.
//
// `build/index.js` alone is one Node process, and Node is single-threaded, so the
// SSR tier used one of host-2's sixteen cores however busy the box got — measured
// with `perf/k6` at PROFILE=saturation, ~130 req/s against a 204 req/s peak. That
// is what kept filling the accept queue. The workers share one listening socket,
// so the port, the systemd unit and the blue/green layout are unchanged.
//
// Four, not one-per-core: each worker is a full copy of the server (~370 MB), the
// memory comes out of the page cache Meilisearch lives on, and eight measured
// WORSE at 200 req/s — they compete with ingest, Meili and Postgres for the same
// cores.
import cluster from 'node:cluster';

const workers = Number(process.env.WEB_CLUSTER_WORKERS || 4);

if (workers <= 1) {
  // One worker would add a supervisor process for nothing. Load the server here
  // so WEB_CLUSTER_WORKERS=1 is a genuine escape hatch back to the old shape.
  await import('./build/index.js');
} else if (cluster.isPrimary) {
  console.log(`cluster primary ${process.pid}: forking ${workers} workers`);
  for (let i = 0; i < workers; i++) cluster.fork();

  // Replace a dead worker, or capacity decays back towards the single process
  // this file exists to get away from.
  cluster.on('exit', (worker, code, signal) => {
    console.log(`worker ${worker.process.pid} exited (${signal || code}) — forking a replacement`);
    cluster.fork();
  });
} else {
  await import('./build/index.js');
}
