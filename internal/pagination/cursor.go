package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("invalid pagination cursor")

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func Encode(createdAt time.Time, id string) string {
	body, _ := json.Marshal(Cursor{CreatedAt: createdAt.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(body)
}
func Decode(value string) (Cursor, error) {
	if value == "" {
		return Cursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var cursor Cursor
	if json.Unmarshal(body, &cursor) != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
		return Cursor{}, ErrInvalidCursor
	}
	return cursor, nil
}
