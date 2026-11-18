package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rig struct {
	t    *testing.T
	srv  *httptest.Server
	cmds []string
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t}
	dir := t.TempDir()
	if err := SetPassword(dir, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	be := Backend{
		Status: func() Status { return Status{Server: "s", State: "active"} },
		Rcon: func(c string) (string, error) {
			r.cmds = append(r.cmds, c)
			if c == "players" {
				return "Players connected (1): \n-Bob\n", nil
			}
			return "ok", nil
		},
		Control: func(string) error { return nil },
	}
	r.srv = httptest.NewServer(New(be, dir, "127.0.0.1:0").Handler())
	t.Cleanup(r.srv.Close)
	return r
}

func (r *rig) do(method, path, body, cookie string, hdr map[string]string) (*http.Response, string) {
	req, _ := http.NewRequest(method, r.srv.URL+path, strings.NewReader(body))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if e != nil {
			break
		}
	}
	return resp, sb.String()
}

var xd = map[string]string{"X-Dbolt": "1"}

func (r *rig) login(pw string) (string, int) {
	resp, _ := r.do("POST", "/api/login", `{"password":"`+pw+`"}`, "", xd)
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				r.t.Fatal("session cookie must be HttpOnly and SameSite=Strict")
			}
			return c.Value, resp.StatusCode
		}
	}
	return "", resp.StatusCode
}

func TestAuthRequiredAndLogin(t *testing.T) {
	r := newRig(t)
	if resp, _ := r.do("GET", "/api/status", "", "", nil); resp.StatusCode != 401 {
		t.Fatalf("status without login: %d", resp.StatusCode)
	}
	if _, code := r.login("wrong password!!"); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	tok, code := r.login("correct horse battery")
	if code != 200 || tok == "" {
		t.Fatalf("login: %d", code)
	}
	if resp, _ := r.do("GET", "/api/status", "", tok, nil); resp.StatusCode != 200 {
		t.Fatalf("status with login: %d", resp.StatusCode)
	}
	r.do("POST", "/api/logout", "{}", tok, xd)
	if resp, _ := r.do("GET", "/api/status", "", tok, nil); resp.StatusCode != 401 {
		t.Fatal("session must die on logout")
	}
}

func TestLockoutAfterRepeatedFailures(t *testing.T) {
	r := newRig(t)
	for i := 0; i < maxFails; i++ {
		r.login("nope nope nope nope")
	}
	if _, code := r.login("correct horse battery"); code != 401 {
		t.Fatalf("even the right password must be refused while locked out, got %d", code)
	}
}

func TestActionsAreValidatedAndCSRFGuarded(t *testing.T) {
	r := newRig(t)
	tok, _ := r.login("correct horse battery")

	if resp, _ := r.do("POST", "/api/action", `{"action":"save"}`, tok, nil); resp.StatusCode != 403 {
		t.Fatalf("POST without X-Dbolt must be refused, got %d", resp.StatusCode)
	}
	if resp, _ := r.do("POST", "/api/action", `{"action":"save"}`, tok, map[string]string{"X-Dbolt": "1", "Origin": "http://evil.example"}); resp.StatusCode != 403 {
		t.Fatalf("foreign Origin must be refused, got %d", resp.StatusCode)
	}
	before := len(r.cmds)
	resp, body := r.do("POST", "/api/action", `{"action":"kick","user":"Bob\" -r \"x"}`, tok, xd)
	if resp.StatusCode != 400 || len(r.cmds) != before {
		t.Fatalf("injection must not reach RCON: %d %s %v", resp.StatusCode, body, r.cmds)
	}
	resp, _ = r.do("POST", "/api/action", `{"action":"additem","user":"Bob","item":"Base.Axe","count":2}`, tok, xd)
	if resp.StatusCode != 200 || r.cmds[len(r.cmds)-1] != `additem "Bob" "Base.Axe" 2` {
		t.Fatalf("%d %v", resp.StatusCode, r.cmds)
	}
}

func TestPlayersAndStatic(t *testing.T) {
	r := newRig(t)
	tok, _ := r.login("correct horse battery")
	_, body := r.do("GET", "/api/players", "", tok, nil)
	var p struct {
		Online  bool
		Players []string
	}
	json.Unmarshal([]byte(body), &p)
	if !p.Online || len(p.Players) != 1 || p.Players[0] != "Bob" {
		t.Fatalf("%s", body)
	}
	resp, html := r.do("GET", "/", "", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(html, "Deadbolt") {
		t.Fatalf("static page: %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("CSP header missing")
	}
}

func TestSetPasswordLength(t *testing.T) {
	if SetPassword(t.TempDir(), "short") == nil {
		t.Fatal("short passwords must be refused")
	}
}
