import { describe, expect, it, vi } from 'vitest';
import { InMemoryLogRecordExporter } from '@opentelemetry/sdk-logs';
import { createErrorTelemetry, installErrorListeners } from './errorTelemetry';

describe('error telemetry', () => {
  it('delivers sanitized exceptions with service identity', async () => {
    const exporter = new InMemoryLogRecordExporter();
    const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-web', environment: 'test', version: 'abc', exporter });
    const error = new TypeError('failed https://alice:secret@host.test/path?token=secret#private alice@example.com Bearer abc123');
    error.stack = `${error.message}\n at https://host.test/app.js?token=secret:1:2`;
    telemetry?.capture(error);
    await telemetry?.flush();
    const [record] = exporter.getFinishedLogRecords();
    if (!record) throw new Error('Expected one exported exception');
    expect(record.resource.attributes).toMatchObject({ 'service.name': 'freehire-web', 'service.version': 'abc', 'deployment.environment.name': 'test' });
    expect(record.severityText).toBe('ERROR');
    expect(record.attributes).toMatchObject({ 'exception.type': 'TypeError', 'event.name': 'exception' });
    expect(JSON.stringify(record.attributes)).not.toMatch(/alice|secret|abc123|private/);
    expect(record.attributes['exception.stacktrace']).toContain('app.js');
    await telemetry?.shutdown();
  });
  it('filters expected noise, deduplicates and bounds event volume', async () => {
    const exporter = new InMemoryLogRecordExporter();
    const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-web', exporter });
    telemetry?.capture(new Error('Failed to fetch'));
    telemetry?.capture({ name: 'AbortError', message: 'cancelled' });
    const error = new Error('a defect');
    telemetry?.capture(error);
    telemetry?.capture(error);
    for (let i = 0; i < 100; i++) telemetry?.capture(new Error(`defect ${i}`));
    await telemetry?.flush();
    expect(exporter.getFinishedLogRecords()).toHaveLength(20);
    await telemetry?.shutdown();
  });
  it('is inert without configuration', () => {
    expect(createErrorTelemetry({ service: 'freehire-web' })).toBeUndefined();
  });
});

it.each(['absolute', 'same-origin'])('exports protobuf over HTTP to a %s receiver URL', async (endpointKind) => {
  const { createServer } = await import('node:http');
  let contentType = '';
  let path = '';
  let body = Buffer.alloc(0);
  const server = createServer(async (request, response) => {
    contentType = request.headers['content-type'] || '';
    path = request.url || '';
    for await (const chunk of request) body = Buffer.concat([body, chunk]);
    response.writeHead(200, { 'Content-Type': 'application/x-protobuf' });
    response.end();
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  if (!address || typeof address === 'string') throw new Error('Missing server address');
  const origin = `http://127.0.0.1:${address.port}`;
  vi.stubGlobal('location', new URL(`${origin}/jobs`));
  const endpoint = endpointKind === 'absolute' ? `${origin}/v1/logs` : '/v1/logs';
  const telemetry = createErrorTelemetry({ endpoint, service: 'freehire-web-ssr' });
  try {
    telemetry?.capture(new Error('wire-test-defect'));
    await telemetry?.flush();
    expect(path).toBe('/v1/logs');
    expect(contentType).toBe('application/x-protobuf');
    expect(body[0]).toBe(10);
    expect(body.toString()).toContain('freehire-web-ssr');
    expect(body.toString()).toContain('wire-test-defect');
  } finally {
    await telemetry?.shutdown();
    vi.unstubAllGlobals();
    await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  }
});


it('captures error and rejection events only when their provenance belongs to this app', async () => {
  const exporter = new InMemoryLogRecordExporter();
  const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-extension', exporter });
  const target = new EventTarget();
  installErrorListeners(target, telemetry, (error, filename) => filename.startsWith('chrome-extension://ours/') || Boolean(error.stack?.includes('chrome-extension://ours/')));
  const host = new Event('error');
  Object.assign(host, { error: new Error('host-page-defect'), filename: 'https://host.test/app.js' });
  target.dispatchEvent(host);
  const own = new Event('error');
  Object.assign(own, { error: new Error('extension-defect'), filename: 'chrome-extension://ours/content.js' });
  target.dispatchEvent(own);
  const rejection = new Event('unhandledrejection');
  const error = new Error('extension-rejection');
  error.stack = 'Error: extension-rejection\n at chrome-extension://ours/content.js:1:1';
  Object.assign(rejection, { reason: error });
  target.dispatchEvent(rejection);
  await telemetry?.flush();
  const records = exporter.getFinishedLogRecords();
  expect(records.map((record) => record.attributes['exception.message'])).toEqual(['extension-defect', 'extension-rejection']);
  await telemetry?.shutdown();
});

it('records primitive rejections and opaque thrown objects without serializing private fields', async () => {
  const exporter = new InMemoryLogRecordExporter();
  const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-web', exporter });
  telemetry?.capture('unexpected alice@example.com token=private');
  telemetry?.capture(42);
  telemetry?.capture({ response: 'private response', auth: 'private credential' });
  await telemetry?.flush();
  const records = exporter.getFinishedLogRecords();
  expect(records.map((record) => record.attributes['exception.type'])).toEqual(['string', 'number', 'object']);
  expect(records[0]?.attributes['exception.message']).toBe('unexpected [email] [redacted]');
  expect(records[1]?.attributes['exception.message']).toBe('42');
  expect(records[2]?.attributes['exception.message']).toBe('Non-Error exception');
  expect(JSON.stringify(records.map((record) => record.attributes))).not.toContain('private');
  await telemetry?.shutdown();
});


it('redacts prefixed, quoted and Basic credential values before export', async () => {
  const exporter = new InMemoryLogRecordExporter();
  const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-web', exporter });
  const error = new Error('access_token=access-private refresh_token:refresh-private client_secret=client-private {"token":"json-private"} authorization: Basic dXNlcjpwYXNz');
  error.stack = error.message;
  telemetry?.capture(error);
  await telemetry?.flush();
  const records = exporter.getFinishedLogRecords();
  expect(records).toHaveLength(1);
  const payload = JSON.stringify(records[0]?.attributes);
  for (const secret of ['access-private', 'refresh-private', 'client-private', 'json-private', 'dXNlcjpwYXNz']) expect(payload).not.toContain(secret);
  await telemetry?.shutdown();
});


it('keeps sanitized local SSR errors when the telemetry volume bound drops records', async () => {
  const local = vi.spyOn(console, 'error').mockImplementation(() => {});
  const exporter = new InMemoryLogRecordExporter();
  const telemetry = createErrorTelemetry({ endpoint: 'http://localhost/v1/logs', service: 'freehire-web-ssr', exporter, logToConsole: true });
  try {
    telemetry?.capture(new Error('Failed to fetch'));
    for (let i = 0; i < 21; i++) {
      const error = new Error(`defect ${i} access_token=local-private authorization: Basic dXNlcjpwYXNz`);
      error.stack = error.message;
      telemetry?.capture(error);
    }
    await telemetry?.flush();
    expect(exporter.getFinishedLogRecords()).toHaveLength(20);
    expect(local).toHaveBeenCalledTimes(21);
    expect(JSON.stringify(local.mock.calls)).not.toMatch(/local-private|dXNlcjpwYXNz/);
  } finally {
    await telemetry?.shutdown();
    local.mockRestore();
  }
});
