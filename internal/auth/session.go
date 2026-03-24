package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const SessionCookieName = "web_claude_session"

type SessionManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewSessionManager(secret string, ttl time.Duration) (*SessionManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("session secret is required")
	}
	return &SessionManager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}, nil
}

func GenerateSessionSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (m *SessionManager) NewCookie(secure bool) (*http.Cookie, error) {
	expiresAt := m.now().Add(m.ttl).Unix()
	payload := strconv.FormatInt(expiresAt, 10)
	signature := base64.RawURLEncoding.EncodeToString(m.sign(payload))

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    payload + "." + signature,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(m.ttl.Seconds()),
		Expires:  time.Unix(expiresAt, 0).UTC(),
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}, nil
}

func (m *SessionManager) ClearCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func (m *SessionManager) IsAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	payload, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok || payload == "" || signature == "" {
		return false
	}

	expected := m.sign(payload)
	actual, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	if !hmac.Equal(actual, expected) {
		return false
	}

	expiresAt, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return m.now().Unix() <= expiresAt
}

func (m *SessionManager) sign(payload string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
