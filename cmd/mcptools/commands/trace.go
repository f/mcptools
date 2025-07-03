package commands

import (
	"context"
	"crypto/rand"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func generateTraceID() [16]byte {
	var traceID [16]byte
	rand.Read(traceID[:])
	return traceID
}

func injectTrace(ctx context.Context) transport.ClientOption {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: generateTraceID(),
		SpanID:  trace.SpanID([8]byte{255}),
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return client.WithHeaders(carrier)
}
