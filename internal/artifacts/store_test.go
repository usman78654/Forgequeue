package artifacts

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignedRequest(t *testing.T) {
	s := Store{AccessKey: "access", SecretKey: "secret"}
	req, err := s.request(context.Background(), http.MethodPut, "https://objects.example.test", "/bucket/key.txt", []byte("hello"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Scheme != "https" || req.URL.Host != "objects.example.test" {
		t.Fatalf("unexpected URL %s", req.URL)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=access/") {
		t.Fatalf("request was not signed: %s", req.Header.Get("Authorization"))
	}
}

func TestPresignedDownload(t *testing.T) {
	s := Store{AccessKey: "access", SecretKey: "secret"}
	raw := s.presign("https://downloads.example.test", "/bucket/job/output.txt", 10*time.Minute)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Query().Get("X-Amz-Expires") != "600" || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("invalid signed URL %s", raw)
	}
}
