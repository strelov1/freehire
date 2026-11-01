package observability

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// The outer process and an inner Bootstrap may both own reporting. The last
// owner closes the provider, so Bootstrap cleanup cannot lose Main's final fault.
var logs struct {
	sync.RWMutex
	provider *sdklog.LoggerProvider
	owners   int
}

// InitLogs enables exception logs only when a standard OTLP endpoint is set.
// Resource environment variables override the supplied defaults. Its idempotent
// cleanup must run after the process's final error or panic has been reported.
func InitLogs(serviceName, environment string) (func(), error) {
	logs.Lock()
	defer logs.Unlock()
	if logs.provider == nil {
		if os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
			return func() {}, nil
		}
		attrs := []attribute.KeyValue{attribute.String("service.name", serviceName)}
		if environment != "" {
			attrs = append(attrs, attribute.String("deployment.environment.name", environment))
		}
		if info, ok := debug.ReadBuildInfo(); ok {
			version := info.Main.Version
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					version = setting.Value
				}
			}
			attrs = append(attrs, attribute.String("service.version", version))
		}
		res, err := resource.New(context.Background(), resource.WithAttributes(attrs...), resource.WithFromEnv())
		if err != nil {
			return nil, fmt.Errorf("exception log resource: %w", err)
		}
		// Local delivery is best effort: batching never blocks a request, retries do
		// not amplify a failed collector, and shutdown has a separate process budget.
		exporter, err := otlploghttp.New(context.Background(), otlploghttp.WithTimeout(time.Second), otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}))
		if err != nil {
			return nil, fmt.Errorf("exception log exporter: %w", err)
		}
		processor := sdklog.NewBatchProcessor(exporter, sdklog.WithMaxQueueSize(256), sdklog.WithExportMaxBatchSize(64), sdklog.WithExportTimeout(time.Second))
		logs.provider = sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(processor))
	}
	logs.owners++
	var once sync.Once
	return func() { once.Do(shutdownLogs) }, nil
}

func shutdownLogs() {
	logs.Lock()
	logs.owners--
	if logs.owners != 0 {
		logs.Unlock()
		return
	}
	provider := logs.provider
	logs.provider = nil
	logs.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	// The SDK's diagnostic handler writes to stderr, never through CaptureError.
	// Export failure must not become another exception or alter process exit status.
	_ = provider.Shutdown(ctx)
}

// CaptureError reports an explicit unexpected failure. Callers classify routine
// client errors before calling; attributes must contain no request/user payloads.
func CaptureError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	if err == nil {
		return
	}
	attrs = append(attrs, attribute.String("error.source", "exception"))
	emitException(ctx, err, otellog.SeverityError, attrs)
}

// CapturePanic reports a recovered panic while its stack is still available.
// The caller decides whether to re-panic or render the normal failure response.
func CapturePanic(ctx context.Context, value any, attrs ...attribute.KeyValue) {
	attrs = append(attrs, attribute.String("error.source", "panic"))
	emitException(ctx, value, otellog.SeverityFatal, attrs)
}

func emitException(ctx context.Context, value any, severity otellog.Severity, attrs []attribute.KeyValue) {
	logs.RLock()
	defer logs.RUnlock()
	if logs.provider == nil {
		return
	}
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetEventName("exception")
	record.SetSeverity(severity)
	if severity == otellog.SeverityFatal {
		record.SetSeverityText("FATAL")
	} else {
		record.SetSeverityText("ERROR")
	}
	message := exceptionMessage(value)
	record.SetBody(attribute.StringValue(message))
	record.AddAttributes(attrs...)
	record.AddAttributes(
		attribute.String("event.name", "exception"),
		attribute.String("exception.type", sanitizeException(fmt.Sprintf("%T", value), 128)),
		attribute.String("exception.message", message),
		attribute.String("exception.stacktrace", sanitizeException(string(debug.Stack()), 8192)),
	)
	logs.provider.Logger("github.com/strelov1/freehire/errors").Emit(ctx, record)
}

func exceptionMessage(value any) string {
	var message string
	switch value := value.(type) {
	case error:
		message = value.Error()
	case string:
		message = value
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		message = fmt.Sprint(value)
	default:
		// A panic may carry a candidate, request, or API response. Keep its type
		// and stack, but never serialize the arbitrary object into telemetry.
		message = "Non-Error exception"
	}
	return sanitizeException(message, 1024)
}

var (
	exceptionURLs    = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s)"'<>]+`)
	exceptionBearer  = regexp.MustCompile(`(?i)\bBearer\s+\S+`)
	exceptionEmails  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	exceptionSecrets = regexp.MustCompile(`(?i)\b[a-z0-9_-]*(?:token|password|passwd|secret|api[_-]?key|authorization|credentials?)["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"?|'(?:\\.|[^'\\])*'?|(?:(?:basic|bearer)\s+)?[^\s,;"']+)`)
	exceptionJWTs    = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
)

// Bound processing as well as output, and remove common credentials from error
// strings (including wrapped url.Errors) and stack filenames before export.
func sanitizeException(value string, limit int) string {
	if len(value) > limit*2 {
		value = value[:limit*2]
	}
	value = exceptionURLs.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[url]"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	})
	value = exceptionBearer.ReplaceAllString(value, "Bearer [redacted]")
	value = exceptionEmails.ReplaceAllString(value, "[email]")
	value = exceptionSecrets.ReplaceAllString(value, "[redacted]")
	value = exceptionJWTs.ReplaceAllString(value, "[token]")
	if len(value) > limit {
		value = value[:limit]
	}
	return strings.ToValidUTF8(value, "")
}
