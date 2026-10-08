package migrations

import "testing"

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
