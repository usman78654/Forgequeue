package migrations

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	items, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("expected at least two migrations, got %d", len(items))
	}
	for i, item := range items {
		if item.SQL == "" || len(item.Checksum) != 64 {
			t.Fatalf("invalid migration %#v", item)
		}
		if i > 0 && items[i-1].Version >= item.Version {
			t.Fatal("migrations are not strictly ordered")
		}
	}
}

func TestDevelopmentTokensRemainConfinedToLegacyMigration(t *testing.T) {
	items, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		for _, token := range developmentPrincipals {
			if item.Version != 7 && strings.Contains(item.SQL, token.Token) {
				t.Fatalf("migration %s introduces development token %q", item.Name, token.Token)
			}
		}
	}
}

func TestLegacyDevelopmentPrincipalsHaveForwardCleanup(t *testing.T) {
	items, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var cleanup string
	for _, item := range items {
		if item.Version > 7 && strings.Contains(item.SQL, "DELETE FROM principals") {
			cleanup += item.SQL
		}
	}
	for _, principal := range developmentPrincipals {
		if !strings.Contains(cleanup, principal.Name) {
			t.Fatalf("no forward migration removes legacy development principal %q", principal.Name)
		}
	}
}
