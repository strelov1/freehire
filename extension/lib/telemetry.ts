import { browser } from 'wxt/browser';
import { createErrorTelemetry, installErrorListeners } from './errorTelemetry';

/** All content-script frames must identify our extension before reporting an error. */
export function ownsExtensionError(error: Error, filename: string, extensionRoot: string): boolean {
  if (filename.startsWith(extensionRoot)) return true;
  return (error.stack || '').split('\n').slice(1).some((frame) => {
    // Match complete source URLs: an extension URL inside a host URL's query
    // does not make that host frame ours.
    const sources = frame.match(/[a-z][a-z0-9+.-]*:\/\/[^\s)]+/gi) || [];
    return sources.some((source) => source.startsWith(extensionRoot));
  });
}

export function startErrorTelemetry(context: 'background' | 'panel' | 'content') {
  const endpoint = import.meta.env.WXT_OTEL_LOGS_ENDPOINT;
  // Content scripts relay through background: a direct page-origin fetch is subject
  // to host-page CORS and would expose telemetry to arbitrary page service workers.
  if (!endpoint || !/^https?:\/\//.test(endpoint)) return;
  const root = browser.runtime.getURL('/');
  if (context === 'content') {
    installErrorListeners(globalThis, { capture(error) {
      if (!(error instanceof Error)) return;
      void browser.runtime.sendMessage({ kind: 'REPORT_EXTENSION_ERROR', name: error.name, message: error.message.slice(0, 1024), stack: error.stack?.slice(0, 8192) }).catch(() => {});
    } }, (error, filename) => ownsExtensionError(error, filename, root));
    return;
  }
  const telemetry = createErrorTelemetry({ endpoint, service: 'freehire-extension', environment: import.meta.env.WXT_OTEL_ENVIRONMENT, version: browser.runtime.getManifest().version });
  installErrorListeners(globalThis, context === 'background' && telemetry ? {
    capture(error) { telemetry.capture(error); void telemetry.flush(); },
  } : telemetry);
  if (context === 'background') {
    browser.runtime.onMessage.addListener((message: unknown, sender) => {
      if (sender.id !== browser.runtime.id || !message || typeof message !== 'object') return;
      const event = message as { kind?: unknown; name?: unknown; message?: unknown; stack?: unknown };
      if (event.kind !== 'REPORT_EXTENSION_ERROR' || typeof event.message !== 'string' || typeof event.stack !== 'string') return;
      const error = new Error(event.message.slice(0, 1024));
      error.name = typeof event.name === 'string' ? event.name.slice(0, 128) : 'Error';
      error.stack = event.stack.slice(0, 8192);
      if (ownsExtensionError(error, '', root)) {
        telemetry?.capture(error);
        // MV3 may suspend the worker as soon as the message finishes.
        return telemetry?.flush();
      }
    });
  }
  return telemetry;
}
