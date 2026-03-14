package telemetry

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/trace"
)

func Log(ctx context.Context, format string, args ...any) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		log.Printf(format, args...)
		return
	}

	prefixArgs := []any{spanContext.TraceID().String(), spanContext.SpanID().String()}
	log.Printf("[trace_id=%s span_id=%s] "+format, append(prefixArgs, args...)...)
}
