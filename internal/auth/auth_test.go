package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookiesUseSecureBrowserAttributes(t *testing.T) {
	recorder := httptest.NewRecorder()
	SetSessionCookie(recorder, "secret", time.Now().Add(time.Hour), true)

	result := recorder.Result()
	cookies := result.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName || cookie.Value != "secret" {
		t.Fatalf("cookie=%+v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("insecure cookie attributes: %+v", cookie)
	}
}

func TestClearSessionCookieExpiresImmediately(t *testing.T) {
	recorder := httptest.NewRecorder()
	ClearSessionCookie(recorder, true)
	cookie := recorder.Result().Cookies()[0]
	if cookie.MaxAge >= 0 || !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("clear cookie attributes: %+v", cookie)
	}
}
