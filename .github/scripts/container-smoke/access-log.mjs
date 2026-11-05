// Receives actual Docker log output on stdin; runs only inside the isolated web image.
import assert from 'node:assert/strict';

let output = '';
for await (const chunk of process.stdin) output += chunk;
// Every sensitive dummy field sent by web.mjs has this prefix. Do not print the log
// on failure: the assertion should never disclose the content it is guarding.
assert.ok(!output.includes('dummy-log-canary-'),
  'dummy query/referrer/cookie/authorization value appeared in container output');
const records = output.split('\n').filter(line => line.startsWith('{"method":'))
  .map(line => JSON.parse(line));
assert.ok(records.some(record => record.method === 'GET'
  && record.path === '/api/privacy-log-canary' && record.status === 200),
'access output must retain method, path and response status');
console.log('PASS: access output retains path/status and excludes query/referrer/cookie/auth canaries');
