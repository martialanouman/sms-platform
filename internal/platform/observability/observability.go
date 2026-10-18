// Package observability builds the structured logger and the OpenTelemetry tracing of a service
// (docs/spec/10-exploitation.md §10.1).
package observability

import (
	"context"
	stderrors "errors"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/martialanouman/sms-platform/internal/platform/errors"
)

// NewLogger returns a JSON logger tagged with service, and with the trace and span of the context when it holds one.
func NewLogger(w io.Writer, service string, level slog.Level) *slog.Logger {
	return slog.New(traceHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})}).With("service", service)
}

type traceHandler struct {
	slog.Handler
}

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// InitTracing sets and returns the global tracer provider of service, exporting to exporter every failed span and
// the successful ones sampled at ratio, and sets TraceContext as the global propagator. debt: #93
func InitTracing(exporter sdktrace.SpanExporter, service string, ratio float64) *sdktrace.TracerProvider {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysRecord(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)))),
		sdktrace.WithSpanProcessor(failedToo{sdktrace.NewBatchSpanProcessor(exporter)}),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(service))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp
}

type failedToo struct {
	sdktrace.SpanProcessor
}

func (p failedToo) OnEnd(s sdktrace.ReadOnlySpan) {
	switch {
	case s.SpanContext().IsSampled():
		p.SpanProcessor.OnEnd(s)
	case s.Status().Code == codes.Error:
		p.SpanProcessor.OnEnd(sampled{s})
	}
}

type sampled struct {
	sdktrace.ReadOnlySpan
}

func (s sampled) SpanContext() trace.SpanContext {
	sc := s.ReadOnlySpan.SpanContext()
	return sc.WithTraceFlags(sc.TraceFlags() | trace.FlagsSampled)
}

// RecordError marks span failed with the code err carries, internal_error otherwise; never with the text of err.
func RecordError(span trace.Span, err error) {
	var c errors.Code
	if !stderrors.As(err, &c) {
		c = errors.ErrInternal
	}
	span.SetStatus(codes.Error, string(c))
	span.RecordError(c)
}
