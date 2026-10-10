// Package worker runs ForgeQueue workers with application-defined handlers.
package worker

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/usman78654/Forgequeue/handler"
	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/jobs"
	internalworker "github.com/usman78654/Forgequeue/internal/worker"
)

// Config contains the process-independent settings for an application worker.
// Database migrations must be applied before Run is called.
type Config struct {
	DatabaseURL  string
	RabbitMQURL  string
	ID           string
	Queues       []string
	Capabilities []string
	Concurrency  int
	Lease        time.Duration
	DrainTimeout time.Duration
	Handlers     *handler.Registry
}

func (c *Config) defaults() {
	if c.ID == "" {
		c.ID, _ = os.Hostname()
	}
	if len(c.Queues) == 0 {
		c.Queues = []string{"jobs.default"}
	}
	if len(c.Capabilities) == 0 && c.Handlers != nil {
		c.Capabilities = c.Handlers.Types()
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.DrainTimeout <= 0 {
		c.DrainTimeout = 45 * time.Second
	}
}

func (c Config) validate() error {
	if c.DatabaseURL == "" {
		return errors.New("worker database URL is required")
	}
	if c.RabbitMQURL == "" {
		return errors.New("worker RabbitMQ URL is required")
	}
	if c.Handlers == nil {
		return errors.New("worker handler registry is required")
	}
	return nil
}

// Run connects to ForgeQueue's PostgreSQL and RabbitMQ services and processes
// jobs until ctx is cancelled. Cancellation begins graceful worker draining.
func Run(ctx context.Context, config Config) error {
	config.defaults()
	if err := config.validate(); err != nil {
		return err
	}
	db, err := database.Open(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	rabbit, err := broker.Open(config.RabbitMQURL)
	if err != nil {
		return err
	}
	defer rabbit.Close()
	runner := internalworker.Worker{
		Store:        jobs.Store{DB: db},
		Rabbit:       rabbit,
		ID:           config.ID,
		Queue:        config.Queues[0],
		Queues:       config.Queues,
		Capabilities: config.Capabilities,
		Concurrency:  config.Concurrency,
		Lease:        config.Lease,
		DrainTimeout: config.DrainTimeout,
		Handlers:     config.Handlers,
	}
	return runner.Run(ctx)
}
