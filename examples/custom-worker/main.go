// Command custom-worker demonstrates an application-defined ForgeQueue worker.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/usman78654/Forgequeue/handler"
	"github.com/usman78654/Forgequeue/worker"
)

type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

type emailResult struct {
	Accepted bool `json:"accepted"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := handler.NewRegistry()
	err := handler.RegisterJSON(registry, "send-email", func(ctx context.Context, request emailRequest, report handler.Reporter) (emailResult, error) {
		if !strings.Contains(request.To, "@") {
			return emailResult{}, handler.Permanent(errors.New("recipient must be an email address"))
		}
		report.Progress(.5, "message accepted by provider")
		select {
		case <-ctx.Done():
			return emailResult{}, ctx.Err()
		default:
		}
		report.Log("info", "email sent", nil)
		return emailResult{Accepted: true}, nil
	})
	if err != nil {
		log.Fatal(err)
	}

	err = worker.Run(ctx, worker.Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
		Queues:      []string{"jobs.email"},
		Handlers:    registry,
	})
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
