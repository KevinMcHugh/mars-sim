package web

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
)

// backdrop is a map color a sprite is previewed on, from
// web/src/map/palette.ts (as the Sprite Designer's BACKDROPS are).
type backdrop struct{ Name, Color string }

var backdrops = []backdrop{{"floor", "#f78765"}, {"rock", "#a8402a"}, {"fog", "#2e0d0b"}}

// tileSizes are the sizes a sprite is previewed at; 16 is where designs fail.
var tileSizes = []int{16, 32, 64}

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{
		"backdrops": func() []backdrop { return backdrops },
		"sizes":     func() []int { return tileSizes },
		"join":      strings.Join,
		"pct":       func(n, d int) int { return 100 * n / max(d, 1) },
	}
	pages := map[string]*template.Template{}
	for _, name := range []string{"login", "index", "preview", "species"} {
		pages[name] = template.Must(template.New("layout.html").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}
	return pages
}

// render writes a page through a buffer, so a template error becomes a 500
// rather than half a page.
func (s *Server) render(w http.ResponseWriter, status int, name string, data map[string]any) {
	var buf bytes.Buffer
	if err := s.pages[name].Execute(&buf, data); err != nil {
		slog.Error("render", "page", name, "err", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// fail answers a page request whose service call failed.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := statusOf(err)
	if status == http.StatusInternalServerError {
		slog.Error("page", "path", r.URL.Path, "err", err)
		http.Error(w, "internal error", status)
		return
	}
	http.Error(w, err.Error(), status)
}

// back redirects to a species page with a one-line message, err or not.
func back(w http.ResponseWriter, r *http.Request, speciesID, anchor string, msg string, isErr bool) {
	key := "msg"
	if isErr {
		key = "err"
	}
	dest := "/species/" + url.PathEscape(speciesID) + "?" + key + "=" + url.QueryEscape(msg)
	if anchor != "" {
		dest += "#" + anchor
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) indexPage(w http.ResponseWriter, r *http.Request) {
	list, err := s.Svc.ListSpecies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	done := 0
	for _, sp := range list {
		if sp.Complete {
			done++
		}
	}
	s.render(w, http.StatusOK, "index", map[string]any{"Species": list, "Complete": done})
}

func (s *Server) previewPage(w http.ResponseWriter, r *http.Request) {
	seed, _ := strconv.ParseInt(r.URL.Query().Get("seed"), 10, 64)
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count == 0 {
		count = 6
	}
	previews := s.Svc.PreviewSpecies(seed, count)
	s.render(w, http.StatusOK, "preview", map[string]any{
		"Previews": previews,
		"Seed":     seed,
		"Count":    len(previews),
		"Next":     previews[len(previews)-1].Seed + 1,
	})
}

func (s *Server) createSpecies(w http.ResponseWriter, r *http.Request) {
	var seed int64
	if v := strings.TrimSpace(r.FormValue("seed")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "seed must be a whole number", http.StatusBadRequest)
			return
		}
		seed = n
	}
	sp, err := s.Svc.CreateSpecies(r.Context(), seed, r.FormValue("notes"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, sp.ID, "", fmt.Sprintf("Saved %s (seed %d).", sp.Traits.Plural, sp.Seed), false)
}

// formSlot is one form's section on the species page.
type formSlot struct {
	creaturelab.FormView
	Accepted   *creaturelab.Candidate
	Candidates []creaturelab.Candidate
}

func (s *Server) speciesPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sp, err := s.Svc.GetSpecies(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cands, err := s.Svc.ListCandidates(r.Context(), id, -1, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	slots := make([]formSlot, len(sp.Forms))
	for i, f := range sp.Forms {
		slots[i].FormView = f
	}
	for _, c := range cands {
		if c.Form >= len(slots) {
			continue
		}
		slots[c.Form].Candidates = append(slots[c.Form].Candidates, c)
	}
	// Point at the accepted one only now the slices are final; append may
	// have moved them.
	for i := range slots {
		for j := range slots[i].Candidates {
			if slots[i].Candidates[j].Accepted {
				slots[i].Accepted = &slots[i].Candidates[j]
			}
		}
	}
	s.render(w, http.StatusOK, "species", map[string]any{
		"Species": sp,
		"Slots":   slots,
		"Msg":     r.URL.Query().Get("msg"),
		"Err":     r.URL.Query().Get("err"),
	})
}

func (s *Server) saveNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.Svc.SetSpeciesNotes(r.Context(), id, r.FormValue("notes")); err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, id, "notes", "Notes saved.", false)
}

func (s *Server) deleteSpecies(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.DeleteSpecies(r.Context(), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// pasteCandidate stores an SVG pasted by hand: drawn in claude.ai from the
// brief, or edited in a vector tool.
func (s *Server) pasteCandidate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	form, ok := formParam(r)
	if !ok {
		http.Error(w, "bad form index", http.StatusBadRequest)
		return
	}
	anchor := fmt.Sprintf("form-%d", form)
	c, err := s.Svc.SubmitCandidate(r.Context(), id, form, r.FormValue("svg"), r.FormValue("note"), r.FormValue("author"), r.FormValue("accept") != "")
	if err != nil {
		if errors.Is(err, creaturelab.ErrBadSVG) || errors.Is(err, creaturelab.ErrNoSuchForm) {
			back(w, r, id, anchor, err.Error(), true)
			return
		}
		s.fail(w, r, err)
		return
	}
	msg := "Candidate added."
	if c.Accepted {
		msg = "Candidate added and accepted."
	}
	if len(c.Warnings) > 0 {
		msg += " Warnings: " + strings.Join(c.Warnings, " ")
	}
	back(w, r, id, anchor, msg, false)
}

func (s *Server) acceptCandidate(w http.ResponseWriter, r *http.Request) {
	c, err := s.Svc.AcceptCandidate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, c.SpeciesID, fmt.Sprintf("form-%d", c.Form), "Sprite accepted.", false)
}

func (s *Server) deleteCandidate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.Svc.GetCandidate(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Svc.DeleteCandidate(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, c.SpeciesID, fmt.Sprintf("form-%d", c.Form), "Candidate removed.", false)
}

// briefText is a form's brief as plain text, to paste into any chat.
func (s *Server) briefText(w http.ResponseWriter, r *http.Request) {
	form, ok := formParam(r)
	if !ok {
		http.Error(w, "bad form index", http.StatusBadRequest)
		return
	}
	brief, err := s.Svc.SpriteBrief(r.Context(), chi.URLParam(r, "id"), form)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(brief + "\n\nReply with exactly one fenced code block tagged svg containing the complete SVG.\n"))
}
