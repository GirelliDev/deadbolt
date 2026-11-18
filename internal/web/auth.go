package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName  = "dbolt_session"
	sessionTTL  = 12 * time.Hour
	maxFails    = 5
	lockoutTime = 5 * time.Minute
)

type auth struct {
	hashFile string
	mu       sync.Mutex
	sessions map[string]time.Time
	fails    map[string]*failState
	now      func() time.Time
}

type failState struct {
	n     int
	until time.Time
}

func newAuth(stateDir string) *auth {
	return &auth{hashFile: filepath.Join(stateDir, "admin.hash"), sessions: map[string]time.Time{}, fails: map[string]*failState{}, now: time.Now}
}

// SetPassword stores a bcrypt hash (file mode 0600) for the single admin.
func SetPassword(stateDir, password string) error {
	if len(password) < 12 {
		return errors.New("password must have at least 12 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "admin.hash"), h, 0o600)
}

func (a *auth) check(ip, password string) error {
	a.mu.Lock()
	f := a.fails[ip]
	if f != nil && a.now().Before(f.until) {
		a.mu.Unlock()
		return errors.New("too many attempts, try again later")
	}
	a.mu.Unlock()

	h, err := os.ReadFile(a.hashFile)
	ok := err == nil && bcrypt.CompareHashAndPassword(h, []byte(password)) == nil
	a.mu.Lock()
	defer a.mu.Unlock()
	if ok {
		delete(a.fails, ip)
		return nil
	}
	if a.fails[ip] == nil {
		a.fails[ip] = &failState{}
	}
	a.fails[ip].n++
	if a.fails[ip].n >= maxFails {
		a.fails[ip].until = a.now().Add(lockoutTime)
		a.fails[ip].n = 0
	}
	return errors.New("wrong password")
}

func (a *auth) newSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	a.mu.Lock()
	defer a.mu.Unlock()
	for t, exp := range a.sessions { // drop expired ones as we go
		if a.now().After(exp) {
			delete(a.sessions, t)
		}
	}
	a.sessions[tok] = a.now().Add(sessionTTL)
	return tok
}

func (a *auth) valid(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for tok, exp := range a.sessions {
		if subtle.ConstantTimeCompare([]byte(tok), []byte(c.Value)) == 1 {
			if a.now().After(exp) {
				delete(a.sessions, tok)
				return false
			}
			return true
		}
	}
	return false
}

func (a *auth) drop(r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
}

// clientIP uses the socket address. X-Forwarded-For is only honoured when the
// panel listens on loopback (i.e. sits behind a local reverse proxy).
func clientIP(r *http.Request, behindProxy bool) string {
	if behindProxy {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			return strings.TrimSpace(strings.Split(xf, ",")[0])
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}
