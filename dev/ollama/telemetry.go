package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
)

func initTracer() (*trace.TracerProvider, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("ollama-proxy"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Set up a connection to the OTLP collector.
	conn, err := grpc.DialContext(ctx, "otel-collector:4317", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	// Set up a trace exporter
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	// Register the trace exporter with a batch processor.
	bsp := trace.NewBatchSpanProcessor(exporter)
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(res),
		trace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tp)

	// Set global text map propagator to tracecontext.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

// LogDataType creates a structured log message with the specified parameters.
func LogDataType(ctx context.Context, msg string, attrs map[string]string) {
	ctx, span := otel.Tracer("ollama-proxy").Start(ctx, "Logging")
	defer span.End()
	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	log.Printf("Trace ID: %s\nSpan ID: %s\nFlags: %d\nLogRecord\nObservedTimestamp: %s\nTimestamp: %s\nSeverityText: %s\nSeverityNumber: %d\nBody: %s\nAttributes:\n",
		traceID, spanID, span.SpanContext().TraceFlags(), time.Now().Format(time.RFC3339),
		time.Now().Format(time.RFC3339), "", 0, msg)

	for key, value := range attrs {
		log.Printf(" -> %s: Str(%s)\n", key, value)
	}
}

func shutdownTracer(tp *trace.TracerProvider) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down tracer provider: %v", err)
	}
}
