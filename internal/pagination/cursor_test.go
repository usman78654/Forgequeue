package pagination

import (
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 29, 1, 2, 3, 4, time.UTC)
	encoded := Encode(at, "abc")
	decoded, err := Decode(encoded)
	if err != nil || !decoded.CreatedAt.Equal(at) || decoded.ID != "abc" {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
}
func TestInvalidCursor(t *testing.T) {
	if _, err := Decode("not-a-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("error=%v", err)
	}
}
