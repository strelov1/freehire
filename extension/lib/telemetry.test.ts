import { describe, expect, it, vi } from 'vitest';
vi.mock('wxt/browser', () => ({ browser: {} }));
import { ownsExtensionError } from './telemetry';

describe('content-script error ownership', () => {
  const root = 'chrome-extension://ours/';
  it('accepts extension script filenames and rejection stack frames', () => {
    expect(ownsExtensionError(new Error('defect'), `${root}content.js`, root)).toBe(true);
    const error = new Error('defect');
    error.stack = `Error: defect\n at ${root}content.js:1:2`;
    expect(ownsExtensionError(error, '', root)).toBe(true);
  });
  it('declines host-page failures even when the message mentions the extension', () => {
    const error = new Error(`host page mentions ${root}`);
    error.stack = `${error.message}\n at https://host.test/app.js:1:2`;
    expect(ownsExtensionError(error, 'https://host.test/app.js', root)).toBe(false);
  });
  it('declines host stack URLs that mention the extension only inside a query', () => {
    const error = new Error('host defect');
    error.stack = `Error: host defect\n at https://host.test/app.js?redirect=${root}content.js:1:2`;
    expect(ownsExtensionError(error, '', root)).toBe(false);
  });
});
