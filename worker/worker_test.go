package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/handler"
)

func TestConfigDefaults(t *testing.T) {
	registry := handler.NewRegistry()
	config := Config{DatabaseURL: "postgres://example", RabbitMQURL: "amqp://example", Handlers: registry}
	config.defaults()
	if config.ID == "" || len(config.Queues) != 1 || config.Queues[0] != "jobs.default" {
		t.Fatalf("unexpected identity or queues: %#v", config)
	}
	if config.Concurrency != 4 || config.Lease != 30*time.Second || config.DrainTimeout != 45*time.Second {
		t.Fatalf("unexpected execution defaults: %#v", config)
	}
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRequiresConnectionsAndRegistry(t *testing.T) {
	tests := []Config{
		{RabbitMQURL: "amqp://example", Handlers: handler.NewRegistry()},
		{DatabaseURL: "postgres://example", Handlers: handler.NewRegistry()},
		{DatabaseURL: "postgres://example", RabbitMQURL: "amqp://example"},
	}
	for _, config := range tests {
		if err := config.validate(); err == nil {
			t.Fatalf("expected validation error for %#v", config)
		}
	}
}

func TestCapabilitiesDefaultToRegisteredTypes(t *testing.T) {
	registry := handler.NewRegistry()
	noop := handler.Func(func(context.Context, handler.Job, handler.Reporter) (json.RawMessage, error) { return nil, nil })
	registry.MustRegister("zeta", noop)
	registry.MustRegister("alpha", noop)
	config := Config{Handlers: registry}
	config.defaults()
	if len(config.Capabilities) != 2 || config.Capabilities[0] != "alpha" || config.Capabilities[1] != "zeta" {
		t.Fatalf("capabilities=%v", config.Capabilities)
	}
}
