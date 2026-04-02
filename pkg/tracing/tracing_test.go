package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestNormalizeTraceIDAcceptsUUIDAndHex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "uuid", input: "550e8400-e29b-41d4-a716-446655440000", want: "550e8400e29b41d4a716446655440000"},
		{name: "hex", input: "550e8400e29b41d4a716446655440000", want: "550e8400e29b41d4a716446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeTraceID(tt.input)
			if !ok {
				t.Fatal("expected trace id to normalize")
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestIDGeneratorUsesPreferredTraceIDFromContext(t *testing.T) {
	ctx, ok := ContextWithTraceID(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
	if !ok {
		t.Fatal("expected context to accept valid trace id")
	}

	traceID, spanID := (idGenerator{}).NewIDs(ctx)
	if traceID.String() != "550e8400e29b41d4a716446655440000" {
		t.Fatalf("expected preferred trace id, got %s", traceID.String())
	}
	if !spanID.IsValid() {
		t.Fatal("expected generated span id to be valid")
	}
}

func TestIDGeneratorFallsBackToRandomTraceID(t *testing.T) {
	traceID, spanID := (idGenerator{}).NewIDs(context.Background())
	if !traceID.IsValid() {
		t.Fatal("expected generated trace id to be valid")
	}
	if !spanID.IsValid() {
		t.Fatal("expected generated span id to be valid")
	}

	if _, err := trace.TraceIDFromHex(traceID.String()); err != nil {
		t.Fatalf("expected generated trace id to be parsable: %v", err)
	}
}
