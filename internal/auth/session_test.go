package auth

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionManagerCookieLifecycle(t *testing.T) {
	manager, err := NewSessionManager("secret-key", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("NewSessionManager returned error: %v", err)
	}
	fixedNow := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return fixedNow }

	cookie, err := manager.NewCookie(false)
	if err != nil {
		t.Fatalf("NewCookie returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if !manager.IsAuthenticated(req) {
		t.Fatal("expected cookie to authenticate request")
	}

	manager.now = func() time.Time { return fixedNow.Add(8 * 24 * time.Hour) }
	if manager.IsAuthenticated(req) {
		t.Fatal("expected expired cookie to be rejected")
	}
}
