import { SeverityNumber } from '@opentelemetry/api';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { BatchLogRecordProcessor, LoggerProvider, type LogRecordExporter } from '@opentelemetry/sdk-logs';
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-proto';
import { isTransientNoise } from './transientNoise.js';

interface ErrorTelemetryOptions {
  endpoint?: string;
  service: string;
  environment?: string;
  version?: string;
  exporter?: LogRecordExporter;
  logToConsole?: boolean;
}

// Only exception fields enter telemetry. Never serialize a thrown object, a request,
// or a rejection value: these often contain API responses or personal input.
function sanitize(value: string, limit: number): string {
  return value
    .slice(0, limit * 2)
    .replace(/(?:https?|file|chrome-extension|moz-extension):\/\/[^\s)"'<>]+/gi, (raw) => {
      try {
        const url = new URL(raw);
        return `${url.protocol}//${url.host}${url.pathname}`;
      } catch {
        return '[url]';
      }
    })
    .replace(/\bBearer\s+\S+/gi, 'Bearer [redacted]')
    .replace(/\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi, '[email]')
    .replace(
      /\b[a-z0-9_-]*(?:token|password|passwd|secret|api[_-]?key|authorization|credentials?)["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"?|'(?:\\.|[^'\\])*'?|(?:(?:basic|bearer)\s+)?[^\s,;"']+)/gi,
      '[redacted]',
    )
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, '[token]')
    .slice(0, limit);
}

export function createErrorTelemetry(options: ErrorTelemetryOptions) {
  if (!options.endpoint) return undefined;
  let provider: LoggerProvider;
  try {
    provider = new LoggerProvider({
      resource: resourceFromAttributes({
        'service.name': options.service,
        'service.version': options.version || 'unknown',
        'deployment.environment.name': options.environment || 'development',
      }),
      logRecordLimits: {
        attributeCountLimit: 8,
        attributeValueLengthLimit: 8192,
      },
      processors: [
        new BatchLogRecordProcessor({
          exporter:
            options.exporter ??
            new OTLPLogExporter({
              url: options.endpoint,
              timeoutMillis: 3000,
              concurrencyLimit: 1,
            }),
          maxQueueSize: 20,
          maxExportBatchSize: 5,
          scheduledDelayMillis: 500,
          exportTimeoutMillis: 3000,
        }),
      ],
    });
  } catch {
    return undefined;
  }
  const logger = provider.getLogger('freehire.errors');
  const seen = new Set<string>();
  let windowStart = Date.now();
  return {
    // `kitError`: the caller vouches this is SvelteKit's { status, message } and not an
    // arbitrary object (a rejection reason may look the same and carry personal data).
    capture(error: unknown, context: { route?: string | null; method?: string; kitError?: boolean } = {}) {
      try {
        if (isTransientNoise(error)) return;
        if (error instanceof Error && /\[['"]@context['"]\]\.toLowerCase/.test(error.message)) return;
        let rawMessage = 'Non-Error exception';
        let type: string = typeof error;
        if (error instanceof Error) {
          rawMessage = error.message;
          type = error.name;
        } else if (['string', 'number', 'boolean', 'bigint'].includes(typeof error)) rawMessage = String(error);
        else if (context.kitError && isHttpError(error)) {
          // SvelteKit hands handleError an App.Error ({ status, message }), not an Error;
          // its message is the literal a load passed to error(), so it is safe to report.
          const detail = error.message;
          rawMessage = `HTTP ${error.status}${typeof detail === 'string' ? `: ${detail}` : ''}`;
          type = 'HttpError';
        }
        const route = context.route ? sanitize(context.route, 256) : '';
        const method = context.method ? sanitize(context.method, 16) : '';
        const message = sanitize(rawMessage, 1024);
        const stack = sanitize(error instanceof Error ? error.stack || '' : '', 8192);
        // Preserve a sanitized local copy before delivery bounds can drop an event.
        if (options.logToConsole) {
          try {
            const where = method || route ? ` [${[method, route].filter(Boolean).join(' ')}]` : '';
            console.error(`${stack || `${sanitize(type, 128)}: ${message}`}${where}`);
          } catch {
            /* Continue exporting if local logging fails. */
          }
        }
        const key = `${route}\n${message}\n${stack}`;
        const now = Date.now();
        if (now - windowStart >= 60_000) {
          windowStart = now;
          seen.clear();
        }
        if (seen.size >= 20 || seen.has(key)) return;
        seen.add(key);
        logger.emit({
          severityNumber: SeverityNumber.ERROR,
          severityText: 'ERROR',
          body: 'Unhandled exception',
          attributes: {
            'event.name': 'exception',
            'exception.type': sanitize(type, 128),
            'exception.message': message,
            'exception.stacktrace': stack,
            ...(route && { 'http.route': route }),
            ...(method && { 'http.request.method': method }),
          },
        });
      } catch {
        /* Reporting must never become an application error. */
      }
    },
    async flush() {
      try {
        await provider.forceFlush({ timeoutMillis: 3000 });
      } catch {
        /* Best effort. */
      }
    },
    async shutdown() {
      try {
        await provider.shutdown();
      } catch {
        /* Best effort. */
      }
    },
  };
}

function isHttpError(error: unknown): error is { status: number; message?: unknown } {
  return typeof error === 'object' && error !== null && typeof (error as { status?: unknown }).status === 'number';
}

type ErrorTelemetry = ReturnType<typeof createErrorTelemetry>;

// Only discard stacks whose every recognizable frame belongs to an extension.
// V8 uses "at fn (url)"; Firefox and Safari use "fn@url".
const EXTENSION_FRAME = /^(?:chrome|moz|safari|safari-web|edge)-extension:\/\//i;
function fromExtensionOnly(error: unknown): boolean {
  if (!(error instanceof Error) || typeof error.stack !== 'string') return false;
  const frames = error.stack.split('\n').filter((line) => /^\s*at\s+|@\S+:\d+/.test(line));
  return (
    frames.length > 0 &&
    frames.every((line) => {
      const location = line
        .trim()
        .replace(/^at\s+(?:.*?\s+\()?/, '')
        .replace(/^.*?@/, '');
      return EXTENSION_FRAME.test(location);
    })
  );
}

/** A provenance predicate is required in content scripts sharing a host page. */
export function installErrorListeners(
  target: EventTarget,
  telemetry: Pick<NonNullable<ErrorTelemetry>, 'capture'> | undefined,
  owns?: (error: Error, filename: string) => boolean,
) {
  // The extension-only filter serves the caller with no predicate of its own (the web
  // client, where every window event is adopted by default). A caller that passes `owns`
  // already answers provenance — for a browser extension the extension's own frames ARE
  // its code — so its answer stands alone.
  const foreign = owns ? undefined : fromExtensionOnly;
  target.addEventListener('error', (event) => {
    const failure = event as ErrorEvent;
    if (failure.error == null) return;
    if (foreign?.(failure.error)) return;
    if (!owns || (failure.error instanceof Error && owns(failure.error, failure.filename || '')))
      telemetry?.capture(failure.error);
  });
  target.addEventListener('unhandledrejection', (event) => {
    const reason: unknown = (event as PromiseRejectionEvent).reason;
    if (foreign?.(reason)) return;
    if (!owns || (reason instanceof Error && owns(reason, ''))) telemetry?.capture(reason);
  });
}
