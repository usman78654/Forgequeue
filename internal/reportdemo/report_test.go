package reportdemo

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/usman78654/Forgequeue/handler"
)

func TestConcurrentReportWrites(t *testing.T) {
	summary, err := Aggregate(context.Background(), fixtureInput(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := WriteReport(context.Background(), directory, summary); err != nil {
				t.Errorf("concurrent write: %v", err)
			}
		}()
	}
	group.Wait()
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one report: %v err=%v", files, err)
	}
}

func fixtureInput(t *testing.T) AggregateInput {
	t.Helper()
	var input AggregateInput
	customers, _ := fixtures.ReadFile("fixtures/customers.json")
	orders, _ := fixtures.ReadFile("fixtures/orders.json")
	if err := json.Unmarshal(customers, &input.Customers); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(orders, &input.Orders); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestAggregateFixtureAndRetrySafeReport(t *testing.T) {
	summary, err := Aggregate(context.Background(), fixtureInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if summary.OrderCount != 5 || summary.TotalCents != 48500 || len(summary.Customers) != 3 || summary.Customers[0].TotalCents != 17000 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	directory := t.TempDir()
	first, err := WriteReport(context.Background(), directory, summary)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteReport(context.Background(), directory, summary)
	if err != nil || first != second {
		t.Fatalf("retry duplicated report: %s %s %v", first, second, err)
	}
	content, err := os.ReadFile(first)
	if err != nil || !strings.Contains(string(content), "Total sales: $485.00") || !strings.Contains(string(content), "Northwind Books | North | 2 | $170.00") {
		t.Fatalf("unexpected report: %s err=%v", content, err)
	}
	files, _ := os.ReadDir(directory)
	if len(files) != 1 {
		t.Fatalf("temporary files remained: %v", files)
	}
}

func TestValidationFailuresArePermanent(t *testing.T) {
	for _, scenario := range []string{"unknown-customer", "duplicate-order", "negative-total", "duplicate-customer", "overflow", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			input := fixtureInput(t)
			switch scenario {
			case "unknown-customer":
				input.Orders[0].CustomerID = "missing"
			case "duplicate-order":
				input.Orders[1].ID = input.Orders[0].ID
			case "negative-total":
				input.Orders[0].TotalCents = -1
			case "duplicate-customer":
				input.Customers[1].ID = input.Customers[0].ID
			case "overflow":
				input.Orders[0].TotalCents = math.MaxInt64
			case "empty":
				input.Orders = nil
			}
			if _, err := Aggregate(context.Background(), input); !handler.IsPermanent(err) {
				t.Fatalf("expected permanent validation failure: %v", err)
			}
		})
	}
}

func TestCancellationAndStrictHandlerPayloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Aggregate(ctx, fixtureInput(t)); err != context.Canceled {
		t.Fatalf("cancelled aggregate: %v", err)
	}
	directory := t.TempDir()
	if _, err := WriteReport(ctx, directory, Summary{}); err != context.Canceled {
		t.Fatalf("cancelled write: %v", err)
	}
	registry, err := Registry(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), handler.Job{Type: "report.read-orders", Payload: json.RawMessage(`{"unexpected":true}`)}, nil); !handler.IsPermanent(err) {
		t.Fatalf("unknown field accepted: %v", err)
	}
	if _, err := registry.Execute(context.Background(), handler.Job{Type: "report.render", Payload: json.RawMessage(`{"summary":{"customers":[],"total_cents":10}}`)}, nil); !handler.IsPermanent(err) {
		t.Fatalf("invalid summary accepted: %v", err)
	}
}
