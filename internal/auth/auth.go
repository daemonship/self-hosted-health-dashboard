package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "hd_session"
	sessionDuration   = 24 * time.Hour
)

type session struct {
	createdAt time.Time
}

// Store is a thread-safe in-memory session store.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]session
}

func NewStore() *Store {
	s := &Store{sessions: make(map[string]session)}
	go s.cleanup()
	return s
}

func (s *Store) cleanup() {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		for token, sess := range s.sessions {
			if time.Since(sess.createdAt) > sessionDuration {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Store) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = session{createdAt: time.Now()}
	s.mu.Unlock()
	return token, nil
}

func (s *Store) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	return ok && time.Since(sess.createdAt) < sessionDuration
}

func (s *Store) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// CheckPassword verifies a submitted password against the stored credential.
// If stored starts with "$2a$" it is treated as a bcrypt hash; otherwise a
// plaintext comparison is used (development only — log a warning upstream).
func CheckPassword(stored, submitted string) bool {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(submitted)) == nil
	}
	// Plaintext fallback — acceptable only in dev environments.
	return stored == submitted
}

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
}

func GetSessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
