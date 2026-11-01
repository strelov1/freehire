import { SeverityNumber } from '@opentelemetry/api';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { BatchLogRecordProcessor, LoggerProvider, type LogRecordExporter } from '@opentelemetry/sdk-logs';
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-proto';
import { isTransientNoise } from './errorNoise.js';

interface ErrorTelemetryOptions {
  endpoint?: string;
  service: string;
  environment?: string;
  version?: string;
  exporter?: LogRecordExporter;
}

// Only exception fields enter telemetry. Never serialize a thrown object, a request,
// or a rejection value: these often contain API responses or personal input.
function sanitize(value: string, limit: number): string {
  return value.slice(0, limit * 2)
    .replace(/(?:https?|file|chrome-extension|moz-extension):\/\/[^\s)"'<>]+/gi, (raw) => {
      try {
        const url = new URL(raw);
        return `${url.protocol}//${url.host}${url.pathname}`;
      } catch { return '[url]'; }
    })
    .replace(/\bBearer\s+\S+/gi, 'Bearer [redacted]')
    .replace(/\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi, '[email]')
    .replace(/\b[a-z0-9_-]*(?:token|password|passwd|secret|api[_-]?key|authorization|credentials?)["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"?|'(?:\\.|[^'\\])*'?|(?:(?:basic|bearer)\s+)?[^\s,;"']+)/gi, '[redacted]')
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
      logRecordLimits: { attributeCountLimit: 8, attributeValueLengthLimit: 8192 },
      processors: [new BatchLogRecordProcessor({
        exporter: options.exporter ?? new OTLPLogExporter({ url: options.endpoint, timeoutMillis: 3000, concurrencyLimit: 1 }),
        maxQueueSize: 20, maxExportBatchSize: 5, scheduledDelayMillis: 500, exportTimeoutMillis: 3000,
      })],
    });
  } catch { return undefined; }
  const logger = provider.getLogger('freehire.errors');
  const seen = new Set<string>();
  let windowStart = Date.now();
  return {
    capture(error: unknown) {
      try {
        if (isTransientNoise(error)) return;
        if (error instanceof Error && /\[['"]@context['"]\]\.toLowerCase/.test(error.message)) return;
        let rawMessage = 'Non-Error exception';
        if (error instanceof Error) rawMessage = error.message;
        else if (['string', 'number', 'boolean', 'bigint'].includes(typeof error)) rawMessage = String(error);
        const type = error instanceof Error ? error.name : typeof error;
        const message = sanitize(rawMessage, 1024);
        const stack = sanitize(error instanceof Error ? error.stack || '' : '', 8192);
        const key = `${message}\n${stack}`;
        const now = Date.now();
        if (now - windowStart >= 60_000) {
          windowStart = now;
          seen.clear();
        }
        if (seen.size >= 20 || seen.has(key)) return;
        seen.add(key);
        logger.emit({
          severityNumber: SeverityNumber.ERROR, severityText: 'ERROR', body: 'Unhandled exception',
          attributes: {
            'event.name': 'exception',
            'exception.type': sanitize(type, 128),
            'exception.message': message,
            'exception.stacktrace': stack,
          },
        });
      } catch { /* Reporting must never become an application error. */ }
    },
    async flush() { try { await provider.forceFlush({ timeoutMillis: 3000 }); } catch { /* Best effort. */ } },
    async shutdown() { try { await provider.shutdown(); } catch { /* Best effort. */ } },
  };
}

type ErrorTelemetry = ReturnType<typeof createErrorTelemetry>;

/** A provenance predicate is required in content scripts sharing a host page. */
export function installErrorListeners(target: EventTarget, telemetry: Pick<NonNullable<ErrorTelemetry>, 'capture'> | undefined, owns?: (error: Error, filename: string) => boolean) {
  target.addEventListener('error', (event) => {
    const failure = event as ErrorEvent;
    if (failure.error != null && (!owns || (failure.error instanceof Error && owns(failure.error, failure.filename || '')))) telemetry?.capture(failure.error);
  });
  target.addEventListener('unhandledrejection', (event) => {
    const reason: unknown = (event as PromiseRejectionEvent).reason;
    if (!owns || (reason instanceof Error && owns(reason, ''))) telemetry?.capture(reason);
  });
}
