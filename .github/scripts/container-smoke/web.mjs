// Runs only inside the network-disabled, read-only web image in container-smoke.yml.
import assert from 'node:assert/strict';
import { access, readFile, readdir, writeFile } from 'node:fs/promises';
import { constants } from 'node:fs';
import { createServer, request } from 'node:http';
import { execFileSync } from 'node:child_process';

assert.notEqual(process.getuid(), 0);
await assert.rejects(access('/app', constants.W_OK));
const configPath = '/tmp/freehire-web/default.conf';
const config = await readFile(configPath, 'utf8');
const nameserver = (await readFile('/etc/resolv.conf', 'utf8')).match(/^nameserver\s+(\S+)/m)[1];
assert.ok(config.includes(`resolver ${nameserver} `), 'runtime DNS substitution');
for (const name of ['client_temp', 'proxy_temp', 'fastcgi_temp', 'uwsgi_temp', 'scgi_temp']) {
  await access(`/tmp/freehire-web/${name}`, constants.W_OK);
}
await access('/tmp/freehire-web/nginx.pid', constants.R_OK);

const backend = createServer(async (req, res) => {
  let bytes = 0;
  for await (const chunk of req) bytes += chunk.length;
  res.setHeader('content-type', 'application/json');
  res.end(JSON.stringify({ headers: req.headers, bytes }));
});
backend.on('upgrade', (req, socket) => {
  assert.equal(req.headers.upgrade, 'websocket');
  assert.equal(req.headers.connection, 'upgrade');
  assert.equal(req.headers.cookie, 'session=dummy');
  assert.equal(req.headers.authorization, 'Bearer dummy');
  socket.end('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n');
});
await new Promise(resolve => backend.listen(18080, '127.0.0.1', resolve));
// Only the rendered scratch copy changes: the real entrypoint/config were already tested.
// Numeric loopback keeps the dummy upstream inside this network-disabled container.
await writeFile(configPath, config.replaceAll('http://app:8080', 'http://127.0.0.1:18080'));
execFileSync('nginx', ['-e', 'stderr', '-s', 'reload']);

function get(path, headers = {}, body) {
  return new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port: 8080, path, headers,
      method: body ? 'POST' : 'GET', timeout: 3000 }, res => {
      let text = '';
      res.setEncoding('utf8');
      res.on('data', chunk => { text += chunk; });
      res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, text }));
    });
    req.on('upgrade', (res, socket) => {
      socket.destroy();
      resolve({ status: res.statusCode, headers: res.headers });
    });
    req.on('error', reject);
    req.on('timeout', () => req.destroy(new Error('request timeout')));
    req.end(body);
  });
}

try {
  // Wait for nginx to replace its worker after reload, rather than racing it.
  let ready = false;
  for (let i = 0; i < 50; i++) {
    try { ready = (await get('/health')).status === 200; } catch { /* worker reloading */ }
    if (ready) break;
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert.ok(ready, 'dummy API route reachable after reload');
  const robots = await get('/robots.txt');
  assert.equal(robots.status, 200);
  assert.match(robots.text, /User-agent:/i);
  assert.equal(robots.headers['x-content-type-options'], 'nosniff');
  const files = await readdir('/app/build/client/_app/immutable', { recursive: true });
  const asset = files.find(file => /\.(js|css)$/.test(file));
  assert.ok(asset, 'real build includes immutable assets');
  const staticResponse = await get(`/_app/immutable/${asset}`);
  assert.equal(staticResponse.status, 200);
  assert.match(staticResponse.headers['cache-control'], /immutable/);
  const body = 'x'.repeat(128 * 1024);
  const posted = await get('/api/smoke', { 'content-type': 'text/plain' }, body);
  assert.equal(posted.status, 200);
  assert.equal(JSON.parse(posted.text).bytes, body.length);
  for (const value of ['websocket', 'WebSocket']) {
    const response = await get('/api/v1/tools/ws', { Upgrade: value, Connection: 'upgrade',
      Cookie: 'session=dummy', Authorization: 'Bearer dummy' });
    assert.equal(response.status, 101);
    assert.equal(response.headers.upgrade, 'websocket');
  }
  // Header checks against a dummy HTTP server, not an h2c implementation or live target.
  for (const value of ['', 'h2c', 'unknown', 'websocket, h2c']) {
    const response = await get('/api/v1/tools/ws', { Upgrade: value, Connection: 'upgrade',
      Cookie: 'session=dummy', Authorization: 'Bearer dummy' });
    assert.equal(response.status, 200);
    const { headers } = JSON.parse(response.text);
    assert.equal(headers.upgrade, undefined);
    assert.equal(headers.connection, 'close');
    assert.equal(headers.cookie, 'session=dummy');
    assert.equal(headers.authorization, 'Bearer dummy');
  }
  const canary = field => `dummy-log-canary-${field}`;
  const privacyResponse = await get(`/api/privacy-log-canary?code=${canary('code')}&state=${canary('state')}&q=${canary('query')}`, {
    Referer: `https://dummy.invalid/?credential=${canary('referrer')}`,
    Cookie: `session=${canary('cookie')}`,
    Authorization: `Bearer ${canary('authorization')}`,
  });
  assert.equal(privacyResponse.status, 200);
  console.log('PASS: non-root web startup, runtime paths, DNS, SSR, static/API routes and websocket allowlist');
} finally {
  backend.closeAllConnections();
  await new Promise(resolve => backend.close(resolve));
}
