package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	handlersdk "github.com/usman78654/Forgequeue/handler"
)

func defaultHandlers(allowCommand bool) *handlersdk.Registry {
	registry := handlersdk.NewRegistry()
	registry.MustRegister("echo", handlersdk.Func(func(_ context.Context, job handlersdk.Job, report handlersdk.Reporter) (json.RawMessage, error) {
		report.Log("info", "echo payload processed", nil)
		return job.Payload, nil
	}))
	type sleepInput struct {
		Duration string `json:"duration"`
	}
	type sleepOutput struct {
		Slept bool `json:"slept"`
	}
	if err := handlersdk.RegisterJSON(registry, "sleep", func(ctx context.Context, input sleepInput, report handlersdk.Reporter) (sleepOutput, error) {
		d, err := time.ParseDuration(input.Duration)
		if err != nil {
			return sleepOutput{}, handlersdk.Permanent(err)
		}
		if d <= 0 {
			return sleepOutput{Slept: true}, nil
		}
		interval := d / 10
		if interval < 250*time.Millisecond {
			interval = 250 * time.Millisecond
		}
		if interval > 2*time.Second {
			interval = 2 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		timer := time.NewTimer(d)
		defer timer.Stop()
		started := time.Now()
		for {
			select {
			case <-ctx.Done():
				return sleepOutput{}, ctx.Err()
			case <-ticker.C:
				value := time.Since(started).Seconds() / d.Seconds()
				if value > .99 {
					value = .99
				}
				report.Progress(value, "sleep progress")
			case <-timer.C:
				return sleepOutput{Slept: true}, nil
			}
		}
	}); err != nil {
		panic(err)
	}
	if allowCommand {
		type commandInput struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		type commandOutput struct {
			Output string `json:"output"`
		}
		if err := handlersdk.RegisterJSON(registry, "command", func(ctx context.Context, input commandInput, report handlersdk.Reporter) (commandOutput, error) {
			if input.Command == "" {
				return commandOutput{}, handlersdk.Permanent(errors.New("command is required"))
			}
			out, err := exec.CommandContext(ctx, input.Command, input.Args...).CombinedOutput()
			if err != nil {
				report.Log("error", "command failed", logData(out))
				return commandOutput{}, fmt.Errorf("%w: %s", err, out)
			}
			report.Log("info", "command completed", logData(out))
			return commandOutput{Output: string(out)}, nil
		}); err != nil {
			panic(err)
		}
	}
	return registry
}

func logData(output []byte) json.RawMessage {
	const max = 16 * 1024
	if len(output) > max {
		output = output[:max]
	}
	body, _ := json.Marshal(map[string]string{"output": string(output)})
	return body
}
