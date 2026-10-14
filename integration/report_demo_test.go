package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/handler"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/reportdemo"
	"github.com/usman78654/Forgequeue/internal/workflows"
)

func TestReportDemoWorkflowExamples(t *testing.T) {
	for _, scenario := range []struct{ file, expected string }{
		{"report-workflow.json", "succeeded"},
		{"report-invalid-workflow.json", "failed"},
	} {
		t.Run(scenario.file, func(t *testing.T) {
			ctx, db := database(t)
			body, err := os.ReadFile(filepath.Join("..", "examples", scenario.file))
			if err != nil {
				t.Fatal(err)
			}
			var input workflows.SubmitRequest
			if err := json.Unmarshal(body, &input); err != nil {
				t.Fatal(err)
			}
			queue := unique("report-integration")
			for i := range input.Steps {
				input.Steps[i].Queue = queue
			}
			workflowStore := workflows.Store{DB: db}
			jobStore := jobs.Store{DB: db}
			workflow, err := workflowStore.Submit(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, workflow.ID)
				db.Exec(ctx, `DELETE FROM jobs WHERE queue=$1`, queue)
			}()
			directory := t.TempDir()
			registry, err := reportdemo.Registry(directory)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := workflowStore.Get(ctx, workflow.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.expected == "succeeded" {
				ready := 0
				for _, step := range initial.Steps {
					if step.JobID != nil {
						ready++
					}
				}
				if ready != 2 {
					t.Fatalf("expected two independent root jobs, got %d", ready)
				}
			}
			for iteration := 0; iteration < 10; iteration++ {
				current, err := workflowStore.Get(ctx, workflow.ID)
				if err != nil {
					t.Fatal(err)
				}
				if current.Status == "succeeded" || current.Status == "failed" {
					break
				}
				processed := false
				for _, step := range current.Steps {
					if step.JobID == nil {
						continue
					}
					job, err := jobStore.Get(ctx, *step.JobID)
					if err != nil {
						t.Fatal(err)
					}
					if job.Status != "queued" {
						continue
					}
					job, err = jobStore.Claim(ctx, job.ID, "report-demo-test", time.Minute)
					if err != nil {
						t.Fatal(err)
					}
					result, executionErr := registry.Execute(ctx, handler.Job{ID: job.ID, Type: job.Type, Queue: job.Queue, Attempt: job.Attempt, Payload: job.Payload}, nil)
					if executionErr != nil {
						err = jobStore.Fail(ctx, job, executionErr)
					} else {
						err = jobStore.Succeed(ctx, job, result)
					}
					if err != nil {
						t.Fatal(err)
					}
					processed = true
				}
				if !processed {
					t.Fatal("workflow stalled without queued jobs")
				}
			}
			final, err := workflowStore.Get(ctx, workflow.ID)
			if err != nil || final.Status != scenario.expected {
				t.Fatalf("workflow=%+v err=%v", final, err)
			}
			files, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.expected == "succeeded" && len(files) != 1 {
				t.Fatalf("expected one generated report, got %d", len(files))
			}
			if scenario.expected == "failed" {
				if len(files) != 0 {
					t.Fatal("invalid workflow generated a report")
				}
				for _, step := range final.Steps {
					if step.Key == "aggregate" && step.JobID != nil {
						job, err := jobStore.Get(ctx, *step.JobID)
						if err != nil || job.Attempt != 1 || job.Status != "dead" {
							t.Fatalf("validation retried: job=%+v err=%v", job, err)
						}
					}
					if step.Key == "report" && step.JobID != nil {
						t.Fatal("blocked report was dispatched")
					}
				}
			}
		})
	}
}
