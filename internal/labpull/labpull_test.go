package labpull

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/web"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

const sprite = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><circle cx="64" cy="64" r="50" fill="#9c6" stroke="#1d1512" stroke-width="6"/></svg>`

// fakeLab serves the public reads for species rolled from seeds, the way a
// lab on this build would, with spoil applied to each species' detail.
func fakeLab(t *testing.T, seeds []int64, spoil func(seed int64, d map[string]any)) *httptest.Server {
	t.Helper()
	details := map[string]map[string]any{}
	var list []map[string]any
	for _, seed := range seeds {
		sp := sim.RollLabSpecies(seed)
		id := fmt.Sprint("s", seed)
		var forms []map[string]any
		for i, name := range formNames(sp) {
			forms = append(forms, map[string]any{"index": i, "name": name, "acceptedId": fmt.Sprintf("%s-%d", id, i)})
		}
		d := map[string]any{
			"id": id, "seed": seed, "generatorRev": "abc",
			"traits": map[string]any{"singular": sp.Singular, "plural": sp.Plural, "scientificName": sp.ScientificName,
				"temperament": sp.Temperament.String(), "apex": sp.Apex},
			"forms": forms,
		}
		if spoil != nil {
			spoil(seed, d)
		}
		details[id] = d
		list = append(list, map[string]any{"id": id, "complete": true})
	}
	list = append(list, map[string]any{"id": "unfinished", "complete": false})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/species", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"species": list})
	})
	mux.HandleFunc("GET /api/species/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(details[r.PathValue("id")])
	})
	mux.HandleFunc("GET /api/species/{id}/candidates", func(w http.ResponseWriter, r *http.Request) {
		var cands []map[string]any
		for _, f := range details[r.PathValue("id")]["forms"].([]map[string]any) {
			cands = append(cands, map[string]any{"id": f["acceptedId"], "form": f["index"], "svg": sprite, "accepted": true})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": cands})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestPullKeepsSpeciesThisBuildRollsAlike(t *testing.T) {
	seeds := []int64{3, 5, 9}
	ts := fakeLab(t, seeds, func(seed int64, d map[string]any) {
		if seed == 5 { // the lab rolled seed 5 into something else
			d["traits"].(map[string]any)["plural"] = "zorbs"
		}
		if seed == 9 { // and never accepted a sprite for its first form
			d["forms"].([]map[string]any)[0]["acceptedId"] = ""
		}
	})
	res, err := Pull(context.Background(), ts.Client(), ts.URL+"/") // trailing slash and all
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pack.Species) != 1 || res.Pack.Species[0].Seed != 3 {
		t.Fatalf("packed %+v, want seed 3 alone", res.Pack.Species)
	}
	ps := res.Pack.Species[0]
	if ps.Species != sim.RollLabSpecies(3) || len(ps.Sprites) != len(formNames(ps.Species)) || ps.Sprites[0].SVG != sprite {
		t.Fatalf("seed 3 packed wrong: %+v", ps)
	}
	skipped := strings.Join(res.Skipped, "\n")
	if len(res.Skipped) != 2 || !strings.Contains(skipped, `"zorbs"; this build rolls`) || !strings.Contains(skipped, "no accepted sprite") {
		t.Fatalf("skipped %q", res.Skipped)
	}
	if res.Incomplete != 1 {
		t.Fatalf("incomplete %d, want 1", res.Incomplete)
	}
	data, err := Encode(res.Pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sim.LoadSpeciesPack(data, "pulled"); err != nil {
		t.Fatal(err)
	}
}

func TestPullErrors(t *testing.T) {
	if _, err := Encode(sim.SpeciesPack{}); !errors.Is(err, ErrNothing) {
		t.Fatalf("an empty pull encoded: %v", err)
	}
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer locked.Close()
	if _, err := Pull(context.Background(), locked.Client(), locked.URL); err == nil || !strings.Contains(err.Error(), "public reads") {
		t.Fatalf("a lab wanting a key: %v", err)
	}
	if _, err := Pull(context.Background(), nil, "lab.example"); err == nil {
		t.Fatal("a URL with no scheme pulled")
	}
	for in, want := range map[string]string{
		"https://lab.example":          "https://lab.example/api",
		"https://lab.example/api/":     "https://lab.example/api",
		"http://localhost:8080/lab/":   "http://localhost:8080/lab/api",
		"https://lab.example/lab/api ": "https://lab.example/lab/api",
	} {
		if got, err := apiRoot(in); err != nil || got != want {
			t.Errorf("apiRoot(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// Against a real Creature Lab (Postgres in CREATURE_LAB_TEST_DATABASE_URL,
// in a schema of its own so it can run beside the lab's own tests): species
// made and drawn with a key, then pulled with none.
func TestPullFromARealLab(t *testing.T) {
	dsn := os.Getenv("CREATURE_LAB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CREATURE_LAB_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS labpull_test CASCADE; CREATE SCHEMA labpull_test;`); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", "labpull_test")
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	files, _ := filepath.Glob("../../creature-lab/db/migrations/*.sql")
	sort.Strings(files)
	for _, f := range files {
		b, _ := os.ReadFile(f)
		up, _, _ := strings.Cut(string(b), "-- migrate:down")
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}

	ts := httptest.NewServer(nil)
	defer ts.Close()
	store := creaturelab.NewStore(pool)
	ts.Config.Handler = web.New(store, ts.URL).Handler()
	raw, _ := auth.GenerateKey()
	if _, err := store.CreateAPIKey(ctx, db.CreateAPIKeyParams{ID: xid.New().String(), Name: "test", KeyHash: auth.Hash(raw)}); err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any, out any) {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, ts.URL+"/api"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 300 {
			t.Fatalf("%s %s: %v %v", method, path, resp.Status, err)
		}
		defer resp.Body.Close()
		if out != nil {
			_ = json.NewDecoder(resp.Body).Decode(out)
		}
	}

	// A species with a life, drawn in full; one single-form, left undrawn.
	var lifeSeed int64
	for s := int64(1); ; s++ {
		if sim.RollLabSpecies(s).FormCount >= 2 {
			lifeSeed = s
			break
		}
	}
	var drawn, undrawn struct {
		ID    string `json:"id"`
		Forms []any  `json:"forms"`
	}
	call("POST", "/species", map[string]any{"seed": lifeSeed}, &drawn)
	call("POST", "/species", map[string]any{"seed": 1}, &undrawn)
	for f := range drawn.Forms {
		call("POST", fmt.Sprintf("/species/%s/forms/%d/candidates", drawn.ID, f), map[string]any{"svg": sprite, "accept": true}, nil)
	}

	res, err := Pull(ctx, ts.Client(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pack.Species) != 1 || res.Incomplete != 1 || len(res.Skipped) != 0 {
		t.Fatalf("pulled %d species, %d incomplete, skipped %q", len(res.Pack.Species), res.Incomplete, res.Skipped)
	}
	ps := res.Pack.Species[0]
	if ps.ID != drawn.ID || ps.Seed != lifeSeed || len(ps.Sprites) != len(drawn.Forms) || ps.Sprites[1].SVG != sprite {
		t.Fatalf("packed %+v", ps)
	}
	if res.Pack.Source != ts.URL {
		t.Fatalf("source %q, want %q", res.Pack.Source, ts.URL)
	}
}
