package server_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server"
)

// fakeStore answers the public reads from memory. Any other query
// panics through the nil Querier, which is the point: a private operation
// must be refused before it reaches the store.
type fakeStore struct {
	db.Querier
	species   db.Species
	candidate db.SpriteCandidate // never accepted: unaccepted candidates are public too
}

func (f fakeStore) AcceptCandidate(context.Context, string) (db.SpriteCandidate, error) {
	panic("not public")
}

func (f fakeStore) ListSpecies(context.Context) ([]db.ListSpeciesRow, error) {
	s := f.species
	return []db.ListSpeciesRow{{ID: s.ID, Seed: s.Seed, Plural: s.Plural, FormCount: s.FormCount, Data: s.Data}}, nil
}

func (f fakeStore) ListAllAccepted(context.Context) ([]db.SpriteCandidate, error) { return nil, nil }

func (f fakeStore) GetSpecies(_ context.Context, id string) (db.Species, error) {
	if id != f.species.ID {
		return db.Species{}, pgx.ErrNoRows
	}
	return f.species, nil
}

func (f fakeStore) ListCandidates(context.Context, string) ([]db.SpriteCandidate, error) {
	return []db.SpriteCandidate{f.candidate}, nil
}

func (f fakeStore) GetCandidate(_ context.Context, id string) (db.SpriteCandidate, error) {
	if id != f.candidate.ID {
		return db.SpriteCandidate{}, pgx.ErrNoRows
	}
	return f.candidate, nil
}

func TestOnlyCatalogReadsArePublic(t *testing.T) {
	got := slices.Sorted(maps.Keys(server.PublicOperations()))
	want := []string{"GET /candidates/{candidateId}", "GET /species", "GET /species/{speciesId}", "GET /species/{speciesId}/candidates"}
	if !slices.Equal(got, want) {
		t.Fatalf("public operations %v, want %v", got, want)
	}
}

func TestAnonymousCallers(t *testing.T) {
	sp := creaturelab.Roll(5)
	data, _ := json.Marshal(sp)
	store := fakeStore{
		species:   db.Species{ID: "sp1", Seed: 5, Plural: sp.Plural, FormCount: int32(len(creaturelab.Forms(sp))), Data: data},
		candidate: db.SpriteCandidate{ID: "c1", SpeciesID: "sp1", Svg: "<svg/>"},
	}

	r := chi.NewRouter()
	deny := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	}
	server.New(store, creaturelab.NewLinks("https://lab.example")).Mount(r, deny)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/species", http.StatusOK},
		{"GET", "/api/species/sp1", http.StatusOK},
		{"GET", "/api/species/nope", http.StatusNotFound},
		{"POST", "/api/species", http.StatusUnauthorized},
		{"PATCH", "/api/species/sp1", http.StatusUnauthorized},
		{"DELETE", "/api/species/sp1", http.StatusUnauthorized},
		{"GET", "/api/preview", http.StatusUnauthorized},
		{"GET", "/api/export", http.StatusUnauthorized},
		{"GET", "/api/species/sp1/candidates", http.StatusOK},
		{"GET", "/api/species/sp1/candidates?form=0&svg=true", http.StatusOK},
		{"GET", "/api/candidates/c1", http.StatusOK},
		{"GET", "/api/candidates/nope", http.StatusNotFound},
		{"GET", "/api/species/sp1/forms/0/brief", http.StatusUnauthorized},
		{"POST", "/api/species/sp1/forms/0/candidates", http.StatusUnauthorized},
		{"POST", "/api/candidates/c1/accept", http.StatusUnauthorized},
		{"DELETE", "/api/candidates/c1", http.StatusUnauthorized},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if rec.Code != tc.want {
			t.Errorf("%s %s: %d, want %d: %s", tc.method, tc.path, rec.Code, tc.want, rec.Body)
		}
		// Public reads open to any origin; nothing else does.
		cors := rec.Header().Get("Access-Control-Allow-Origin")
		if public := tc.want != http.StatusUnauthorized; public != (cors == "*") {
			t.Errorf("%s %s: Access-Control-Allow-Origin %q", tc.method, tc.path, cors)
		}
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/species/sp1", nil))
	var detail struct {
		ID     string
		Traits struct{ Plural string }
		Forms  []struct{ Name string }
		URL    string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != "sp1" || detail.Traits.Plural != sp.Plural || len(detail.Forms) != len(creaturelab.Forms(sp)) || detail.URL != "https://lab.example/species/sp1" {
		t.Fatalf("detail %+v", detail)
	}
}
