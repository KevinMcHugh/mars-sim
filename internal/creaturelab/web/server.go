// Package web is Creature Lab's HTTP surface: the JSON API, the MCP
// endpoint's mount, OAuth, and a handful of server-rendered pages for looking
// at sprites and accepting them. The pages are plain html/template with no
// script, so there is no frontend build: the binary is the whole deploy.
package web

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/labmcp"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/oauth"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server holds what the routes need.
type Server struct {
	Svc    *creaturelab.Service
	OAuth  *oauth.Handler
	Issuer string       // public origin, no trailing slash
	MCP    http.Handler // the streamable MCP handler, mounted at /mcp/rpc

	pages map[string]*template.Template
}

// New assembles the server on svc: OAuth issuing for issuer, and the MCP
// tools over the same service.
func New(svc *creaturelab.Service, issuer string) *Server {
	mcpServer := labmcp.NewServer(svc)
	return &Server{
		Svc:    svc,
		OAuth:  &oauth.Handler{Issuer: issuer, Q: svc.Q},
		Issuer: issuer,
		MCP:    mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return mcpServer }, nil),
	}
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	s.pages = parsePages()
	q := s.Svc.Q
	denyAPI := auth.Unauthorized(s.Issuer)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	s.OAuth.Mount(r)
	r.Get("/login", s.loginGET)
	r.With(s.sameOrigin).Post("/login", s.loginPOST)
	r.With(s.sameOrigin).Post("/logout", s.logout)

	r.Route("/api", func(r chi.Router) {
		r.Use(auth.Require(q, denyAPI))
		s.mountAPI(r)
	})

	// The bare /mcp path is intercepted by the sprites.dev proxy (a POST
	// hangs before reaching the app), so MCP lives at /mcp/rpc, as in the
	// inventory app.
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(q, denyAPI))
		r.Handle("/mcp/rpc", s.MCP)
		r.Handle("/mcp/rpc/*", s.MCP)
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(q, s.toLogin))
		r.Use(s.sameOrigin)
		r.Use(pageHeaders)
		r.Get("/", s.indexPage)
		r.Get("/preview", s.previewPage)
		r.Post("/species", s.createSpecies)
		r.Get("/species/{id}", s.speciesPage)
		r.Post("/species/{id}/notes", s.saveNotes)
		r.Post("/species/{id}/delete", s.deleteSpecies)
		r.Post("/species/{id}/forms/{form}/candidates", s.pasteCandidate)
		r.Post("/candidates/{id}/accept", s.acceptCandidate)
		r.Post("/candidates/{id}/delete", s.deleteCandidate)
		r.Get("/brief/{id}/{form}", s.briefText)
	})
	// Sprites take a bearer token as well as the session cookie, so the
	// svgUrl in an API or MCP response opens for a client too.
	r.With(auth.Require(q, denyAPI)).Get("/sprites/{id}.svg", s.spriteSVG)
	return r
}

// ---- middleware -------------------------------------------------------------

// toLogin is the deny for pages: send the browser to /login and back.
func (s *Server) toLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
}

// sameOrigin refuses a state-changing request whose Origin header names
// another site. The session cookie is SameSite=Lax, which already keeps it
// off cross-site POSTs in current browsers; this is the second lock.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "null" && o != s.Issuer && o != requestOrigin(r) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requestOrigin is the origin the request was addressed to, for local runs
// where PUBLIC_URL is not set to the address in the browser.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// pageHeaders forbids every script on the lab's pages. Nothing on them needs
// one, and a sprite is text a model wrote.
func pageHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// ---- login ------------------------------------------------------------------

func (s *Server) loginGET(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "login", map[string]any{"Next": safeNext(r.URL.Query().Get("next"))})
}

func (s *Server) loginPOST(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.FormValue("next"))
	raw := strings.TrimSpace(r.FormValue("api_key"))
	key, err := s.Svc.Q.GetAPIKeyByHash(r.Context(), auth.Hash(raw))
	if err != nil {
		status, msg := http.StatusUnauthorized, "That api key is not recognized."
		if !errors.Is(err, pgx.ErrNoRows) {
			status, msg = http.StatusInternalServerError, "Sign-in failed; try again."
		}
		s.render(w, status, "login", map[string]any{"Next": next, "Error": msg})
		return
	}
	token, err := s.OAuth.MintSession(r.Context(), key.ID)
	if err != nil {
		http.Error(w, "session failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(oauth.SessionTTL / time.Second),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.Issuer, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = s.OAuth.RevokeSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// safeNext keeps a post-login redirect on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// ---- sprites ----------------------------------------------------------------

// spriteSVG serves one candidate as an image. A candidate never changes
// after it is stored, so it caches for a day. The sandbox CSP means a
// browser opening it directly runs nothing even if CheckSVG ever let
// something through.
func (s *Server) spriteSVG(w http.ResponseWriter, r *http.Request) {
	svg, err := s.Svc.CandidateSVG(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, creaturelab.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("sprite", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write([]byte(svg))
}

// ---- helpers ----------------------------------------------------------------

func formParam(r *http.Request) (int, bool) {
	f, err := strconv.Atoi(chi.URLParam(r, "form"))
	return f, err == nil && f >= 0
}
