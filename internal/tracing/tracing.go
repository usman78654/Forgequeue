package tracing

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.26.0"
	"net/http"
	"os"
)

func Init(ctx context.Context, service string) (func(context.Context) error, error) {
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(service))), sdktrace.WithSampler(sdktrace.AlwaysSample())}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown, nil
}
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("forgequeue/http").Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func Traceparent(ctx context.Context) string {
	header := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
	return header.Get("traceparent")
}
func Context(traceparent string) context.Context {
	header := http.Header{}
	if traceparent != "" {
		header.Set("traceparent", traceparent)
	}
	return otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(header))
}
