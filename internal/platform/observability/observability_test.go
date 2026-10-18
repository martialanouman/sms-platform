package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
	"github.com/martialanouman/sms-platform/internal/platform/msg"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
)

func TestLoggerWritesJSONWithServiceAndTrace(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := observability.InitTracing(exp, "router-svc", 1)
	ctx, span := tp.Tracer("t").Start(context.Background(), "step")
	defer span.End()

	var w bytes.Buffer
	log := observability.NewLogger(&w, "router-svc", slog.LevelWarn)
	log.InfoContext(ctx, "hidden")
	log.WarnContext(ctx, "routed")
	log.Warn("no span")

	lines := strings.Split(strings.TrimSpace(w.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (info filtered): %s", len(lines), w.String())
	}
	var first, second map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	sc := span.SpanContext()
	if first["msg"] != "routed" || first["service"] != "router-svc" || first["trace_id"] != sc.TraceID().String() || first["span_id"] != sc.SpanID().String() {
		t.Errorf("first line = %v", first)
	}
	if _, ok := second["trace_id"]; ok || second["service"] != "router-svc" {
		t.Errorf("second line = %v", second)
	}
	var grouped bytes.Buffer
	observability.NewLogger(&grouped, "s", slog.LevelInfo).With("k", "v").WithGroup("g").InfoContext(ctx, "m", "a", 1)
	if !strings.Contains(grouped.String(), sc.TraceID().String()) {
		t.Errorf("With and WithGroup dropped the trace: %s", grouped.String())
	}
}

func spans(t *testing.T, ratio float64, run func(trace.Tracer)) tracetest.SpanStubs {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := observability.InitTracing(exp, "router-svc", ratio)
	run(otel.Tracer("t"))
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	return exp.GetSpans()
}

func TestAFailedSpanIsAlwaysExported(t *testing.T) {
	got := spans(t, 0, func(tr trace.Tracer) {
		_, ok := tr.Start(context.Background(), "ok")
		ok.End()
		_, failed := tr.Start(context.Background(), "failed")
		observability.RecordError(failed, fmt.Errorf("route: %w", errs.ErrNoRoute))
		failed.End()
	})
	if len(got) != 1 || got[0].Name != "failed" || !got[0].SpanContext.IsSampled() {
		t.Fatalf("exported %v, want only the failed span, sampled", got)
	}
	if got[0].Status.Code != codes.Error || got[0].Status.Description != "no_route" {
		t.Errorf("status = %+v", got[0].Status)
	}
	if !got[0].Resource.Set().HasValue("service.name") {
		t.Errorf("resource = %v", got[0].Resource)
	}
}

func TestSuccessIsSampledByRatioAndParent(t *testing.T) {
	if got := spans(t, 1, func(tr trace.Tracer) {
		_, s := tr.Start(context.Background(), "ok")
		s.End()
	}); len(got) != 1 {
		t.Errorf("ratio 1 exported %d spans, want 1", len(got))
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	if got := spans(t, 0, func(tr trace.Tracer) {
		_, s := tr.Start(trace.ContextWithRemoteSpanContext(context.Background(), parent), "child")
		s.End()
	}); len(got) != 1 {
		t.Errorf("a child of a sampled parent exported %d spans, want 1", len(got))
	}
}

func TestRecordErrorCarriesTheCodeOnly(t *testing.T) {
	got := spans(t, 1, func(tr trace.Tracer) {
		_, s := tr.Start(context.Background(), "failed")
		observability.RecordError(s, fmt.Errorf("dial host=db.internal for +2250700000000"))
		s.End()
	})
	if len(got) != 1 || got[0].Status.Description != "internal_error" {
		t.Fatalf("exported %v", got)
	}
	out := fmt.Sprintf("%v", got[0])
	for _, leak := range []string{"db.internal", "2250700000000"} {
		if strings.Contains(out, leak) {
			t.Errorf("span carries %q: %s", leak, out)
		}
	}
	if len(got[0].Events) != 1 || !strings.Contains(fmt.Sprint(got[0].Events[0].Attributes), "internal_error") {
		t.Errorf("events = %v", got[0].Events)
	}
}

func TestI1BodyNeverLeaksIntoASpan(t *testing.T) {
	b := msg.NewBody([]byte("hello secret"))
	got := spans(t, 1, func(tr trace.Tracer) {
		_, s := tr.Start(context.Background(), "step", trace.WithAttributes(attribute.String("body", fmt.Sprint(b))))
		observability.RecordError(s, fmt.Errorf("refused %v: %w", b, errs.ErrContentBlocked))
		s.End()
	})
	if out := fmt.Sprintf("%+v", got); strings.Contains(out, "secret") {
		t.Errorf("span leaks the body: %s", out)
	}
}

func TestPropagatorIsTraceContextOnly(t *testing.T) {
	observability.InitTracing(tracetest.NewInMemoryExporter(), "s", 1)
	if _, ok := otel.GetTextMapPropagator().(propagation.TraceContext); !ok {
		t.Errorf("propagator = %T", otel.GetTextMapPropagator())
	}
}
