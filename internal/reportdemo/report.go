// Package reportdemo implements real, offline report-generation demo handlers.
package reportdemo

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/usman78654/Forgequeue/handler"
)

//go:embed fixtures/*.json
var fixtures embed.FS

type Customer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
}
type Order struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	TotalCents int64  `json:"total_cents"`
}
type Sales struct {
	Customer   Customer `json:"customer"`
	Orders     int      `json:"orders"`
	TotalCents int64    `json:"total_cents"`
}
type Summary struct {
	Customers  []Sales `json:"customers"`
	OrderCount int     `json:"order_count"`
	TotalCents int64   `json:"total_cents"`
}
type AggregateInput struct {
	Customers []Customer `json:"customers"`
	Orders    []Order    `json:"orders"`
}

// Aggregate validates foreign keys and duplicate IDs before computing totals.
// Money uses integer cents, not floating-point arithmetic.
func Aggregate(ctx context.Context, input AggregateInput) (Summary, error) {
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	if len(input.Customers) == 0 || len(input.Orders) == 0 {
		return Summary{}, handler.Permanent(fmt.Errorf("customers and orders must not be empty"))
	}
	byID := make(map[string]*Sales)
	for _, customer := range input.Customers {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if customer.ID == "" || strings.TrimSpace(customer.Name) == "" || strings.TrimSpace(customer.Region) == "" {
			return Summary{}, handler.Permanent(fmt.Errorf("customer requires id, name and region"))
		}
		if _, exists := byID[customer.ID]; exists {
			return Summary{}, handler.Permanent(fmt.Errorf("duplicate customer %q", customer.ID))
		}
		byID[customer.ID] = &Sales{Customer: customer}
	}
	seen := make(map[string]bool)
	summary := Summary{Customers: make([]Sales, 0, len(byID))}
	for _, order := range input.Orders {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if order.ID == "" || seen[order.ID] || order.TotalCents < 0 {
			return Summary{}, handler.Permanent(fmt.Errorf("order IDs must be unique and amounts nonnegative"))
		}
		seen[order.ID] = true
		sales, exists := byID[order.CustomerID]
		if !exists {
			return Summary{}, handler.Permanent(fmt.Errorf("order %q refers to unknown customer %q", order.ID, order.CustomerID))
		}
		if summary.TotalCents > math.MaxInt64-order.TotalCents {
			return Summary{}, handler.Permanent(fmt.Errorf("sales total exceeds integer range"))
		}
		sales.Orders++
		sales.TotalCents += order.TotalCents
		summary.OrderCount++
		summary.TotalCents += order.TotalCents
	}
	for _, sales := range byID {
		summary.Customers = append(summary.Customers, *sales)
	}
	sort.Slice(summary.Customers, func(i, j int) bool { return summary.Customers[i].Customer.ID < summary.Customers[j].Customer.ID })
	return summary, nil
}

func money(cents int64) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }
func cell(value string) string {
	return strings.NewReplacer("|", "\\|", "\r", " ", "\n", " ").Replace(value)
}

// Render produces deterministic Markdown; no timestamps or external services.
func Render(summary Summary) string {
	var report strings.Builder
	fmt.Fprint(&report, "# Sales report\n\nCurrency: USD (demo data).\n\n")
	fmt.Fprintf(&report, "Orders: %d\n\nTotal sales: %s\n\n", summary.OrderCount, money(summary.TotalCents))
	fmt.Fprintln(&report, "| Customer | Region | Orders | Sales |\n|---|---|---:|---:|")
	for _, sales := range summary.Customers {
		fmt.Fprintf(&report, "| %s | %s | %d | %s |\n", cell(sales.Customer.Name), cell(sales.Customer.Region), sales.Orders, money(sales.TotalCents))
	}
	return report.String()
}

// WriteReport stages complete content before publishing a content-addressed
// filename. Retries with identical inputs target identical bytes, not duplicates.
func WriteReport(ctx context.Context, directory string, summary Summary) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	content := []byte(Render(summary))
	if err := os.MkdirAll(directory, 0750); err != nil {
		return "", err
	}
	filename := filepath.Join(directory, fmt.Sprintf("sales-%x.md", sha256.Sum256(content)))
	if existing, err := os.ReadFile(filename); err == nil {
		if string(existing) != string(content) {
			return "", fmt.Errorf("existing report content does not match digest")
		}
		return filename, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	temp, err := os.CreateTemp(directory, ".sales-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(temp.Name(), filename); err != nil {
		// Another execution may have published identical content while this
		// attempt was staging it (Windows does not replace existing targets).
		if existing, readErr := os.ReadFile(filename); readErr == nil && string(existing) == string(content) {
			return filename, nil
		}
		return "", err
	}
	return filename, nil
}

func Registry(outputDirectory string) (*handler.Registry, error) {
	registry := handler.NewRegistry()
	for _, dataset := range []string{"customers", "orders"} {
		dataset := dataset
		err := handler.RegisterJSON(registry, "report.read-"+dataset, func(ctx context.Context, _ struct{}, reporter handler.Reporter) (map[string]json.RawMessage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			data, err := fixtures.ReadFile("fixtures/" + dataset + ".json")
			if err != nil {
				return nil, handler.Permanent(err)
			}
			reporter.Progress(1, "Loaded bundled "+dataset+" dataset")
			return map[string]json.RawMessage{dataset: data}, nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err := handler.RegisterJSON(registry, "report.aggregate", func(ctx context.Context, input AggregateInput, reporter handler.Reporter) (map[string]Summary, error) {
		reporter.Progress(.25, "Validating customer and order records")
		summary, err := Aggregate(ctx, input)
		if err != nil {
			return nil, err
		}
		reporter.Progress(1, "Validated and aggregated sales")
		return map[string]Summary{"summary": summary}, nil
	}); err != nil {
		return nil, err
	}
	if err := handler.RegisterJSON(registry, "report.render", func(ctx context.Context, input struct {
		Summary Summary `json:"summary"`
	}, reporter handler.Reporter) (map[string]any, error) {
		// Recompute supplied totals so this handler cannot publish an inconsistent report.
		var total int64
		var orders int
		for _, sales := range input.Summary.Customers {
			if sales.TotalCents < 0 || sales.Orders < 0 || total > math.MaxInt64-sales.TotalCents || orders > math.MaxInt-sales.Orders {
				return nil, handler.Permanent(fmt.Errorf("invalid report totals"))
			}
			total += sales.TotalCents
			orders += sales.Orders
		}
		if len(input.Summary.Customers) == 0 || total != input.Summary.TotalCents || orders != input.Summary.OrderCount {
			return nil, handler.Permanent(fmt.Errorf("inconsistent report summary"))
		}
		reporter.Progress(.5, "Rendering Markdown report")
		path, err := WriteReport(ctx, outputDirectory, input.Summary)
		if err != nil {
			return nil, err
		}
		reporter.Log("info", "Report written to local demo output directory", nil)
		reporter.Progress(1, "Report complete")
		return map[string]any{"path": path, "markdown": Render(input.Summary), "total_cents": total, "order_count": orders}, nil
	}); err != nil {
		return nil, err
	}
	return registry, nil
}
