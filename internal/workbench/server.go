// Package workbench serves the AWK workbench: a page on this computer where a
// person who does not write code plans with the council, starts and stops the
// work, and sees what everyone is doing.
//
// It listens on 127.0.0.1 only. Each start makes a new token: the address
// `awkit ui` prints carries it once, the page keeps it in a cookie, and
// nothing under /api answers without it. What the page shows is read from the
// project's own files with the functions the commands use, so the workbench
// and the terminal never disagree.
package workbench

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
	"github.com/silver2dream/ai-workflow-kit/internal/status"
)

// cookieName is the cookie that carries the token.
const cookieName = "awk_workbench"

// mutationHeader is a header every request that changes something carries. A
// page of another site cannot add a header to a request it sends here without
// asking first, and it is never answered when it asks.
const mutationHeader = "X-AWK-Workbench"

// Options configure a workbench server.
type Options struct {
	// Store keeps the person's settings.
	Store *Store
	// Assets are the files of the page; nil when this awkit was built without
	// them.
	Assets fs.FS
	// Version is the version of awkit.
	Version string
	// Now tells the time; time.Now when nil.
	Now func() time.Time
	// IssuesIn asks GitHub for a project's issues, from the project's folder;
	// gh in that folder when nil.
	IssuesIn func(root string) status.IssueLister
	// Poll is how often a stream looks for changes; a second when zero.
	Poll time.Duration
	// SetupProbe asks about what a run of the project in root needs: the
	// checker of awkit run, asking git and gh in root, when nil.
	SetupProbe func(root string) doctor.SetupProbe
}

// Server is the workbench of one awkit process.
type Server struct {
	opts  Options
	token string
	port  int
	mux   *http.ServeMux

	ticketsMu sync.Mutex
	tickets   map[string]ticketCache
}

// New makes a workbench server with a new token.
func New(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, errors.New("the workbench needs a settings store")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.IssuesIn == nil {
		opts.IssuesIn = issuesWithGhIn
	}
	if opts.SetupProbe == nil {
		opts.SetupProbe = doctor.SetupProbeOf
	}
	if opts.Poll <= 0 {
		opts.Poll = time.Second
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	s := &Server{opts: opts, token: hex.EncodeToString(raw), mux: http.NewServeMux(), tickets: map[string]ticketCache{}}
	s.routes()
	return s, nil
}

// Token is the token of this start.
func (s *Server) Token() string { return s.token }

// Listen takes a port on 127.0.0.1: the one asked for, or any free one when
// it is 0 or taken.
func (s *Server) Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return nil, err
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	return ln, nil
}

// URL is the address that opens the workbench, token and all.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/?token=%s", s.port, s.token)
}

// Serve answers on a listener until the context ends.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	// A request lives no longer than the server: a page's stream ends when
	// the workbench closes, rather than holding the close up.
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdown)
		case <-done:
		}
	}()
	err := srv.Serve(ln)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Handler is the workbench, guarded.
func (s *Server) Handler() http.Handler { return s.guard(s.mux) }

// guard lets through only what this person's page sends to this address.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A page of another site that makes its own name point here (DNS
		// rebinding) still sends its own name as the host.
		if !s.hostAllowed(r.Host) {
			http.Error(w, "the workbench answers on 127.0.0.1 and localhost only", http.StatusForbidden)
			return
		}
		if token := r.URL.Query().Get("token"); token != "" {
			if !s.tokenOK(token) {
				http.Error(w, "this address is from an earlier start of the workbench: open the one `awkit ui` printed last", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			// To the page itself, on this address: the page routes the rest,
			// and a path taken from the request could name another site
			// ("//elsewhere").
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			cookie, err := r.Cookie(cookieName)
			if err != nil || !s.tokenOK(cookie.Value) {
				writeError(w, http.StatusUnauthorized, "open the workbench with the address `awkit ui` printed")
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(mutationHeader) != "1" {
				writeError(w, http.StatusForbidden, "a change must come from the workbench's own page")
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) hostAllowed(host string) bool {
	name, port, err := net.SplitHostPort(host)
	if err != nil || (s.port != 0 && port != strconv.Itoa(s.port)) {
		return false
	}
	return name == "127.0.0.1" || name == "localhost"
}

// serveAssets answers with the page's files. An address the page routes
// itself gets the page.
func (s *Server) serveAssets(w http.ResponseWriter, r *http.Request) {
	if s.opts.Assets == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, notBuiltPage)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	file, err := s.opts.Assets.Open(name)
	if err != nil {
		// A file of an earlier build, asked for by a page opened before
		// awkit was upgraded: the page in its place would be read as script.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		file, err = s.opts.Assets.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		// Their names carry a hash of what is in them.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, name, info.ModTime(), file.(io.ReadSeeker))
}

const notBuiltPage = `<!doctype html><meta charset="utf-8"><title>AWK 工作台</title>
<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;line-height:1.6;padding:0 1rem">
<h1>這個 awkit 沒有包含工作台的畫面</h1>
<p>它是在沒有先建置前端的情況下編出來的。請改用發佈的版本，或在 awkit 的原始碼裡先執行 <code>npm --prefix web ci</code> 與 <code>npm --prefix web run build</code>，再編一次。</p>
<p>This awkit was built without the workbench's page. Use a released awkit, or build the page first (<code>npm --prefix web ci && npm --prefix web run build</code>) and build awkit again.</p>`

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

// readJSON reads a request's body into a value; a body is small.
func readJSON(r *http.Request, into any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("the request cannot be read: %w", err)
	}
	return nil
}
