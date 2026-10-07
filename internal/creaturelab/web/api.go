package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
)

// mountAPI attaches the JSON API. It mirrors the MCP tools one for one, for
// scripts and for a game build fetching the export.
func (s *Server) mountAPI(r chi.Router) {
	r.Get("/preview", s.apiPreview)
	r.Get("/export", s.apiExport)
	r.Get("/species", s.apiListSpecies)
	r.Post("/species", s.apiCreateSpecies)
	r.Get("/species/{id}", s.apiGetSpecies)
	r.Patch("/species/{id}", s.apiPatchSpecies)
	r.Delete("/species/{id}", s.apiDeleteSpecies)
	r.Get("/species/{id}/forms/{form}/brief", s.apiBrief)
	r.Get("/species/{id}/candidates", s.apiListCandidates)
	r.Post("/species/{id}/forms/{form}/candidates", s.apiSubmit)
	r.Get("/candidates/{id}", s.apiGetCandidate)
	r.Post("/candidates/{id}/accept", s.apiAccept)
	r.Delete("/candidates/{id}", s.apiDeleteCandidate)
}

func (s *Server) apiPreview(w http.ResponseWriter, r *http.Request) {
	seed, _ := strconv.ParseInt(r.URL.Query().Get("seed"), 10, 64)
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count == 0 {
		count = 3
	}
	writeJSON(w, http.StatusOK, map[string]any{"species": s.Svc.PreviewSpecies(seed, count)})
}

func (s *Server) apiExport(w http.ResponseWriter, r *http.Request) {
	out, err := s.Svc.Export(r.Context(), r.URL.Query().Get("all") != "")
	respond(w, r, out, err)
}

func (s *Server) apiListSpecies(w http.ResponseWriter, r *http.Request) {
	list, err := s.Svc.ListSpecies(r.Context())
	respond(w, r, map[string]any{"species": list}, err)
}

func (s *Server) apiCreateSpecies(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Seed  int64  `json:"seed"`
		Notes string `json:"notes"`
	}
	if !decode(w, r, &in) {
		return
	}
	sp, err := s.Svc.CreateSpecies(r.Context(), in.Seed, in.Notes)
	if err == nil {
		writeJSON(w, http.StatusCreated, sp)
		return
	}
	respond(w, r, nil, err)
}

func (s *Server) apiGetSpecies(w http.ResponseWriter, r *http.Request) {
	sp, err := s.Svc.GetSpecies(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, sp, err)
}

func (s *Server) apiPatchSpecies(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Notes string `json:"notes"`
	}
	if !decode(w, r, &in) {
		return
	}
	sp, err := s.Svc.SetSpeciesNotes(r.Context(), chi.URLParam(r, "id"), in.Notes)
	respond(w, r, sp, err)
}

func (s *Server) apiDeleteSpecies(w http.ResponseWriter, r *http.Request) {
	err := s.Svc.DeleteSpecies(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, map[string]bool{"ok": err == nil}, err)
}

func (s *Server) apiBrief(w http.ResponseWriter, r *http.Request) {
	form, err := strconv.Atoi(chi.URLParam(r, "form"))
	if err != nil {
		apiError(w, http.StatusBadRequest, "form must be a number")
		return
	}
	brief, err := s.Svc.SpriteBrief(r.Context(), chi.URLParam(r, "id"), form)
	respond(w, r, map[string]any{"form": form, "brief": brief}, err)
}

func (s *Server) apiListCandidates(w http.ResponseWriter, r *http.Request) {
	form := -1
	if v := r.URL.Query().Get("form"); v != "" {
		f, err := strconv.Atoi(v)
		if err != nil {
			apiError(w, http.StatusBadRequest, "form must be a number")
			return
		}
		form = f
	}
	list, err := s.Svc.ListCandidates(r.Context(), chi.URLParam(r, "id"), form, r.URL.Query().Get("svg") != "")
	respond(w, r, map[string]any{"candidates": list}, err)
}

func (s *Server) apiSubmit(w http.ResponseWriter, r *http.Request) {
	form, err := strconv.Atoi(chi.URLParam(r, "form"))
	if err != nil {
		apiError(w, http.StatusBadRequest, "form must be a number")
		return
	}
	var in struct {
		SVG    string `json:"svg"`
		Note   string `json:"note"`
		Author string `json:"author"`
		Accept bool   `json:"accept"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := s.Svc.SubmitCandidate(r.Context(), chi.URLParam(r, "id"), form, in.SVG, in.Note, in.Author, in.Accept)
	if err == nil {
		writeJSON(w, http.StatusCreated, c)
		return
	}
	respond(w, r, nil, err)
}

func (s *Server) apiGetCandidate(w http.ResponseWriter, r *http.Request) {
	c, err := s.Svc.GetCandidate(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, c, err)
}

func (s *Server) apiAccept(w http.ResponseWriter, r *http.Request) {
	c, err := s.Svc.AcceptCandidate(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, c, err)
}

func (s *Server) apiDeleteCandidate(w http.ResponseWriter, r *http.Request) {
	err := s.Svc.DeleteCandidate(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, map[string]bool{"ok": err == nil}, err)
}

// ---- helpers ----------------------------------------------------------------

// statusOf maps the service's errors to HTTP statuses.
func statusOf(err error) int {
	switch {
	case errors.Is(err, creaturelab.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, creaturelab.ErrNoSuchForm), errors.Is(err, creaturelab.ErrBadSVG), errors.Is(err, creaturelab.ErrBadInput):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, v)
		return
	}
	status := statusOf(err)
	if status == http.StatusInternalServerError {
		slog.Error("api", "path", r.URL.Path, "err", err)
		apiError(w, status, "internal error")
		return
	}
	apiError(w, status, err.Error())
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*creaturelab.MaxSVGBytes)).Decode(v); err != nil {
		apiError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return false
	}
	return true
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
