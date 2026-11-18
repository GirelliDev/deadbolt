// Package web serves the Deadbolt admin panel: a small JSON API plus a static page.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed static
var staticFS embed.FS

type Status struct {
	Server     string `json:"server"`
	Service    string `json:"service"`
	State      string `json:"state"`
	PublicName string `json:"publicName"`
	Port       string `json:"port"`
	Mods       int    `json:"mods"`
	Items      int    `json:"items"`
}

type BackupRow struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Reason string `json:"reason"`
	Time   string `json:"time"`
}

type UpdateInfo struct {
	Game string   `json:"game"`
	Mods []string `json:"mods"`
	Note string   `json:"note,omitempty"`
}

type ModRow struct {
	ID      string   `json:"id"`
	Mods    []string `json:"mods"`
	Enabled []string `json:"enabled"`
}

// Backend is what the panel needs from the rest of Deadbolt.
type Backend struct {
	Status       func() Status
	Rcon         func(cmd string) (string, error)
	Control      func(action string) error // start | stop | restart
	Backups      func() ([]BackupRow, error)
	CreateBackup func() (string, error)
	Updates      func() (UpdateInfo, error)
	Mods         func() ([]ModRow, error)
}

type Server struct {
	be       Backend
	auth     *auth
	stateDir string
	proxied  bool
	auditMu  sync.Mutex

	updMu   sync.Mutex
	updAt   time.Time
	updLast UpdateInfo
}

func New(be Backend, stateDir, listen string) *Server {
	host, _, _ := net.SplitHostPort(listen)
	ip := net.ParseIP(host)
	return &Server{be: be, auth: newAuth(stateDir), stateDir: stateDir, proxied: host == "localhost" || (ip != nil && ip.IsLoopback())}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/login", s.login)
	mux.HandleFunc("/api/logout", s.guard(s.logout))
	mux.HandleFunc("/api/status", s.guard(s.status))
	mux.HandleFunc("/api/players", s.guard(s.players))
	mux.HandleFunc("/api/action", s.guard(s.action))
	mux.HandleFunc("/api/server", s.guard(s.serverCtl))
	mux.HandleFunc("/api/backups", s.guard(s.backups))
	mux.HandleFunc("/api/updates", s.guard(s.updates))
	mux.HandleFunc("/api/mods", s.guard(s.mods))
	return secure(mux)
}

func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]string{"error": msg}) }

// sameOrigin blocks cross-site POSTs: a custom header (which browsers will not
// send cross-site without CORS) plus a matching Origin when one is present.
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("X-Dbolt") != "1" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		return strings.HasSuffix(o, "://"+r.Host)
	}
	return true
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.valid(r) {
			fail(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if r.Method != http.MethodGet && !sameOrigin(r) {
			fail(w, http.StatusForbidden, "bad origin")
			return
		}
		h(w, r)
	}
}

func (s *Server) audit(r *http.Request, format string, a ...any) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.stateDir, "audit.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s\n", time.Now().Format(time.RFC3339), clientIP(r, s.proxied), fmt.Sprintf(format, a...))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !sameOrigin(r) {
		fail(w, http.StatusForbidden, "bad request")
		return
	}
	var in struct{ Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "bad json")
		return
	}
	ip := clientIP(r, s.proxied)
	if err := s.auth.check(ip, in.Password); err != nil {
		s.audit(r, "login FAILED")
		time.Sleep(500 * time.Millisecond)
		fail(w, http.StatusUnauthorized, err.Error())
		return
	}
	secure := r.TLS != nil || (s.proxied && r.Header.Get("X-Forwarded-Proto") == "https")
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.auth.newSession(), Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: secure, MaxAge: int(sessionTTL.Seconds())})
	s.audit(r, "login ok")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.drop(r)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.be.Status()) }

func (s *Server) players(w http.ResponseWriter, r *http.Request) {
	reply, err := s.be.Rcon("players")
	if err != nil {
		writeJSON(w, 200, map[string]any{"online": false, "error": err.Error(), "players": []string{}})
		return
	}
	p := ParsePlayers(reply)
	if p == nil {
		p = []string{}
	}
	writeJSON(w, 200, map[string]any{"online": true, "players": p})
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var in struct {
		Action string `json:"action"`
		Params
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "bad json")
		return
	}
	cmd, err := BuildCommand(in.Action, in.Params)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "action %s", cmd)
	out, err := s.be.Rcon(cmd)
	if err != nil {
		fail(w, http.StatusBadGateway, "RCON: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"reply": strings.TrimSpace(out)})
}

func (s *Server) serverCtl(w http.ResponseWriter, r *http.Request) {
	var in struct{ Action string }
	if r.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		fail(w, http.StatusBadRequest, "POST {action}")
		return
	}
	if in.Action != "start" && in.Action != "stop" && in.Action != "restart" {
		fail(w, http.StatusBadRequest, "action must be start, stop or restart")
		return
	}
	s.audit(r, "server %s", in.Action)
	if err := s.be.Control(in.Action); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.audit(r, "backup create")
		name, err := s.be.CreateBackup()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"name": name})
		return
	}
	rows, err := s.be.Backups()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []BackupRow{}
	}
	writeJSON(w, 200, rows)
}

// updates is slow (steamcmd + Steam API), so the answer is cached for 5 minutes.
func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	s.updMu.Lock()
	defer s.updMu.Unlock()
	if time.Since(s.updAt) > 5*time.Minute || r.URL.Query().Get("refresh") == "1" {
		u, err := s.be.Updates()
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		s.updLast, s.updAt = u, time.Now()
	}
	writeJSON(w, 200, s.updLast)
}

func (s *Server) mods(w http.ResponseWriter, r *http.Request) {
	rows, err := s.be.Mods()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []ModRow{}
	}
	writeJSON(w, 200, rows)
}
