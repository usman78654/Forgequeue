package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/internal/auth"
	"github.com/usman78654/Forgequeue/internal/httpapi"
)

func TestWebSessionExchangeCSRFAndRevocation(t *testing.T) {
	ctx, db := database(t)
	var principal auth.Principal
	if err := db.QueryRow(ctx, `INSERT INTO principals(name,role) VALUES($1,'operator') RETURNING id,name,role::text`, unique("session")).Scan(&principal.ID, &principal.Name, &principal.Role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(ctx, `DELETE FROM principals WHERE id=$1`, principal.ID) })
	_, apiKey, err := auth.CreateKey(ctx, db, principal.ID, "browser exchange", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := httpapi.New(db, nil).ConfigureWebSessions(time.Hour, true).Handler()
	exchange := httptest.NewRequest(http.MethodPost, "/v1/session", nil)
	exchange.Header.Set("Authorization", "Bearer "+apiKey)
	exchangeResult := httptest.NewRecorder()
	handler.ServeHTTP(exchangeResult, exchange)
	if exchangeResult.Code != http.StatusCreated {
		t.Fatalf("exchange status=%d body=%s", exchangeResult.Code, exchangeResult.Body.String())
	}
	var session auth.WebSession
	if err = json.NewDecoder(exchangeResult.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.CSRFToken == "" || session.Principal.ID != principal.ID {
		t.Fatalf("session=%+v", session)
	}
	cookies := exchangeResult.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie=%+v", cookies)
	}

	missingCSRF := httptest.NewRequest(http.MethodDelete, "/v1/session", nil)
	missingCSRF.AddCookie(cookies[0])
	missingCSRFResult := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFResult, missingCSRF)
	if missingCSRFResult.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", missingCSRFResult.Code)
	}

	revoke := httptest.NewRequest(http.MethodDelete, "/v1/session", nil)
	revoke.AddCookie(cookies[0])
	revoke.Header.Set("X-CSRF-Token", session.CSRFToken)
	revokeResult := httptest.NewRecorder()
	handler.ServeHTTP(revokeResult, revoke)
	if revokeResult.Code != http.StatusNoContent {
		t.Fatalf("revoke status=%d body=%s", revokeResult.Code, revokeResult.Body.String())
	}

	refresh := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
	refresh.AddCookie(cookies[0])
	refreshResult := httptest.NewRecorder()
	handler.ServeHTTP(refreshResult, refresh)
	if refreshResult.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d", refreshResult.Code)
	}
}
