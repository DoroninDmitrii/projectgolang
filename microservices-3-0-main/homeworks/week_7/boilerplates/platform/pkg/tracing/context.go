package tracing

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// TraceIDFromContext извлекает trace ID из текущего спана в контексте
// Возвращает пустую строку, если активного трейса нет (например, TracerProvider
// не был инициализирован через InitTracer — типовой случай для API-тестов)
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}

	return sc.TraceID().String()
}
