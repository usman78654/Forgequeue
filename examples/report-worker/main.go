// Command report-worker runs real offline reporting handlers on jobs.reports.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/usman78654/Forgequeue/internal/reportdemo"
	"github.com/usman78654/Forgequeue/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	output := os.Getenv("REPORT_OUTPUT_DIR")
	if output == "" {
		output = "demo-output/reports"
	}
	registry, err := reportdemo.Registry(output)
	if err != nil {
		log.Fatal(err)
	}
	err = worker.Run(ctx, worker.Config{DatabaseURL: os.Getenv("DATABASE_URL"), RabbitMQURL: os.Getenv("RABBITMQ_URL"), Queues: []string{"jobs.reports"}, Concurrency: 4, Handlers: registry})
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
