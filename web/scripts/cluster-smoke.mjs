import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { cp, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';

// Exercise the production supervisor with real processes and sockets. The fixture
// has adapter-node's signal contract: stop accepting, drain requests, then exit.
const fixture = `
import http from 'node:http';
let stopping = false;
const server = http.createServer((req, res) => {
  if (req.url === '/crash') process.exit(1);
  console.log('request started');
  setTimeout(() => res.end('drained'), 200);
});
server.listen(0, '127.0.0.1', () => console.log('ready ' + server.address().port));
function shutdown() {
  if (stopping) return;
  stopping = true;
  server.close(() => process.emit('sveltekit:shutdown'));
  server.closeIdleConnections();
}
process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);
// Application shutdown hooks, including telemetry, may finish asynchronously.
process.on('sveltekit:shutdown', () => setTimeout(() => console.log('cleanup finished'), 50));
`;

async function start(t, workers = 2) {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'freehire-cluster-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(path.join(dir, 'build'));
  await cp(new URL('../cluster.js', import.meta.url), path.join(dir, 'cluster.js'));
  await writeFile(path.join(dir, 'package.json'), '{"type":"module"}');
  await writeFile(path.join(dir, 'build/index.js'), fixture);
  const child = spawn(process.execPath, ['cluster.js'], {
    cwd: dir,
    env: { ...process.env, WEB_CLUSTER_WORKERS: String(workers) },
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  t.after(() => {
    try {
      process.kill(-child.pid, 'SIGKILL');
    } catch (err) {
      if (err.code !== 'ESRCH') throw err;
    }
  });
  let output = '';
  let errors = '';
  child.stdout.on('data', (data) => { output += data; });
  child.stderr.on('data', (data) => { errors += data; });
  const exited = once(child, 'close');
  async function waitFor(predicate) {
    while (!predicate(output)) {
      assert.equal(child.exitCode, null, `primary exited early: ${errors}`);
      // oxlint-disable-next-line no-await-in-loop -- Readiness depends on the next event.
      await Promise.race([
        once(child.stdout, 'data'),
        exited.then(() => { throw new Error(`primary exited early: ${errors}`); }),
      ]);
    }
  }
  await waitFor((text) => (text.match(/ready /g) || []).length === workers);
  return {
    child, exited, waitFor,
    port: Number(output.match(/ready (\d+)/)[1]),
    output: () => output,
    errors: () => errors,
  };
}

function request(port, route = '/') {
  return new Promise((resolve, reject) => {
    const req = http.get({ host: '127.0.0.1', port, path: route, agent: false }, (res) => {
      let body = '';
      res.on('data', (chunk) => { body += chunk; });
      res.on('end', () => resolve(body));
      res.on('error', reject);
    });
    req.on('error', reject);
  });
}

for (const signal of ['SIGTERM', 'SIGINT']) {
  for (const group of [false, true]) {
    test(`${signal} to ${group ? 'the process group' : 'the primary'} drains requests`, { timeout: 5000 }, async (t) => {
      const app = await start(t);
      const response = request(app.port);
      // Attach the assertion before signaling so a reset is never unhandled.
      const drained = assert.doesNotReject(async () => assert.equal(await response, 'drained'));
      await app.waitFor((text) => text.includes('request started'));
      process.kill(group ? -app.child.pid : app.child.pid, signal);
      await drained;
      assert.deepEqual(await app.exited, [0, null]);
      assert.equal((app.output().match(/cleanup finished/g) || []).length, 2);
      assert.equal(app.errors(), '');
      assert.doesNotMatch(app.output(), /forking a replacement/);
    });
  }
}

test('a crashed worker is replaced while serving', { timeout: 5000 }, async (t) => {
  const app = await start(t);
  await assert.rejects(request(app.port, '/crash'));
  await app.waitFor((text) => (text.match(/ready /g) || []).length === 3);
  assert.equal(await request(app.port), 'drained');
});

test('one worker still runs without a supervisor', { timeout: 5000 }, async (t) => {
  const app = await start(t, 1);
  assert.equal(await request(app.port), 'drained');
  app.child.kill('SIGTERM');
  assert.deepEqual(await app.exited, [0, null]);
  assert.match(app.output(), /cleanup finished/);
  assert.doesNotMatch(app.output(), /cluster primary/);
});
