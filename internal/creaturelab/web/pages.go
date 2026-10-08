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
	apigen "github.com/kevinmchugh/mars-sim/internal/creaturelab/api/gen"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/species"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/sprites"
)

// The pages drive the same endpoints as the JSON API and MCP, through
// server.BuildViewModel, and render the view models as HTML.

// backdrop is a map color a sprite is previewed on, from
// web/src/map/palette.ts (as the Sprite Designer's BACKDROPS are).
type backdrop struct{ Name, Color string }

var backdrops = []backdrop{{"floor", "#f78765"}, {"rock", "#a8402a"}, {"fog", "#2e0d0b"}}

// tileSizes are the sizes a sprite is previewed at; 16 is where designs fail.
var tileSizes = []int{16, 32, 64}

// spritePreview is what the "preview" template draws: one sprite at every
// tile size on every backdrop, beside the emoji it replaces, as the Scum Lab
// Sprite Designer previews a drawing.
type spritePreview struct {
	URL   string
	Emoji string
}

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{
		"backdrops": func() []backdrop { return backdrops },
		"sizes":     func() []int { return tileSizes },
		"join":      strings.Join,
		"pct":       func(n, d int) int { return 100 * n / max(d, 1) },
		"preview":   func(url, emoji string) spritePreview { return spritePreview{URL: url, Emoji: emoji} },
	}
	pages := map[string]*template.Template{}
	for _, name := range []string{"login", "index", "preview", "species"} {
		pages[name] = template.Must(template.New("layout.html").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}
	return pages
}

// render writes a page through a buffer, so a template error becomes a 500
// rather than half a page. Every page learns whether the visitor is signed
// in: the public pages hide their forms from anyone who is not.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]any) {
	data["SignedIn"] = auth.KeyID(r.Context()) != ""
	data["Path"] = r.URL.RequestURI()
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

// statusOf maps the endpoints' errors to HTTP statuses.
func statusOf(err error) int {
	switch {
	case errors.Is(err, creaturelab.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, creaturelab.ErrNoSuchForm), errors.Is(err, creaturelab.ErrBadSVG):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// fail answers a page request whose endpoint failed.
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
	list, err := server.BuildViewModel(r.Context(), species.ListEndpoint{Store: s.Store, Links: s.Links}, apigen.ListSpeciesRequestObject{})
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
	s.render(w, r, http.StatusOK, "index", map[string]any{"Species": list, "Complete": done})
}

func (s *Server) previewPage(w http.ResponseWriter, r *http.Request) {
	seed, _ := strconv.ParseInt(r.URL.Query().Get("seed"), 10, 64)
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count == 0 {
		count = 6
	}
	previews, err := server.BuildViewModel(r.Context(), species.PreviewEndpoint{}, apigen.PreviewSpeciesRequestObject{
		Params: apigen.PreviewSpeciesParams{Seed: seed, Count: count},
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "preview", map[string]any{
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
	sp, err := server.BuildViewModel(r.Context(), species.CreateEndpoint{Store: s.Store, Links: s.Links}, apigen.CreateSpeciesRequestObject{
		Body: &apigen.SpeciesCreate{Seed: seed, Notes: r.FormValue("notes")},
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, sp.ID, "", fmt.Sprintf("Saved %s (seed %d).", sp.Traits.Plural, sp.Seed), false)
}

// formSlot is one form's section on the species page.
type formSlot struct {
	species.FormView
	Accepted   *sprites.Candidate
	Candidates []sprites.Candidate
}

func (s *Server) speciesPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sp, err := server.BuildViewModel(r.Context(), species.GetEndpoint{Store: s.Store, Links: s.Links}, apigen.GetSpeciesRequestObject{SpeciesId: id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cands, err := server.BuildViewModel(r.Context(), sprites.ListEndpoint{Store: s.Store, Links: s.Links}, apigen.ListSpriteCandidatesRequestObject{SpeciesId: id})
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
	s.render(w, r, http.StatusOK, "species", map[string]any{
		"Species": sp,
		"Slots":   slots,
		"Msg":     r.URL.Query().Get("msg"),
		"Err":     r.URL.Query().Get("err"),
	})
}

func (s *Server) saveNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := server.BuildViewModel(r.Context(), species.UpdateEndpoint{Store: s.Store, Links: s.Links}, apigen.UpdateSpeciesRequestObject{
		SpeciesId: id,
		Body:      &apigen.SpeciesUpdate{Notes: r.FormValue("notes")},
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, id, "notes", "Notes saved.", false)
}

func (s *Server) deleteSpecies(w http.ResponseWriter, r *http.Request) {
	if _, err := server.BuildViewModel(r.Context(), species.DeleteEndpoint{Store: s.Store}, apigen.DeleteSpeciesRequestObject{SpeciesId: chi.URLParam(r, "id")}); err != nil {
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
	c, err := server.BuildViewModel(r.Context(), sprites.SubmitEndpoint{Store: s.Store, Links: s.Links}, apigen.SubmitSpriteCandidateRequestObject{
		SpeciesId: id,
		Form:      form,
		Body: &apigen.CandidateSubmit{
			Svg:    r.FormValue("svg"),
			Note:   r.FormValue("note"),
			Author: r.FormValue("author"),
			Accept: r.FormValue("accept") != "",
		},
	})
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
	c, err := server.BuildViewModel(r.Context(), sprites.AcceptEndpoint{Store: s.Store, Links: s.Links}, apigen.AcceptSpriteCandidateRequestObject{CandidateId: chi.URLParam(r, "id")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back(w, r, c.SpeciesID, fmt.Sprintf("form-%d", c.Form), "Sprite accepted.", false)
}

func (s *Server) deleteCandidate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := server.BuildViewModel(r.Context(), sprites.GetEndpoint{Store: s.Store, Links: s.Links}, apigen.GetSpriteCandidateRequestObject{CandidateId: id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := server.BuildViewModel(r.Context(), sprites.DeleteEndpoint{Store: s.Store}, apigen.DeleteSpriteCandidateRequestObject{CandidateId: id}); err != nil {
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
	brief, err := server.BuildViewModel(r.Context(), sprites.BriefEndpoint{Store: s.Store}, apigen.GetSpriteBriefRequestObject{SpeciesId: chi.URLParam(r, "id"), Form: form})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(brief.Brief + "\n\nReply with exactly one fenced code block tagged svg containing the complete SVG.\n"))
}
