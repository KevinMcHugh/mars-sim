// Package labpull pulls a species pack from a Creature Lab server: every
// species with a sprite for every form, as sim.PackedSpecies, using only the
// lab's public reads (no key). See docs/species-pack.md.
//
// The public reads describe a species (its seed, the commit that rolled it,
// its traits and forms) but do not carry the stored sim.AlienSpecies itself;
// only the private /api/export does. So Pull rolls each seed again with this
// build's roster code and keeps the species only if what it rolls is the
// species the lab shows: same names, temperament, apex, and forms. A seed
// rolls differently once the roster code changes, and a species that would
// arrive changed is skipped with the reason, never packed wrong.
package labpull

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// maxBody bounds one response: a species list or one species' candidates.
const maxBody = 16 << 20

// Result is what a pull found.
type Result struct {
	Pack sim.SpeciesPack
	// Skipped says, one line each, which complete species were left out and
	// why.
	Skipped []string
	// Incomplete counts species still missing a sprite for some form.
	Incomplete int
}

// The parts of the lab's public responses a pull reads
// (creature-lab/api/openapi.yaml).
type (
	speciesList struct {
		Species []struct {
			ID       string `json:"id"`
			Complete bool   `json:"complete"`
		} `json:"species"`
	}
	speciesDetail struct {
		ID           string `json:"id"`
		Seed         int64  `json:"seed"`
		GeneratorRev string `json:"generatorRev"`
		Traits       struct {
			Singular       string `json:"singular"`
			Plural         string `json:"plural"`
			ScientificName string `json:"scientificName"`
			Temperament    string `json:"temperament"`
			Apex           bool   `json:"apex"`
		} `json:"traits"`
		Forms []struct {
			Index      int    `json:"index"`
			Name       string `json:"name"`
			AcceptedID string `json:"acceptedId"`
		} `json:"forms"`
	}
	candidateList struct {
		Candidates []struct {
			ID       string `json:"id"`
			Form     int    `json:"form"`
			SVG      string `json:"svg"`
			Accepted bool   `json:"accepted"`
		} `json:"candidates"`
	}
)

// Pull reads the lab at base ("https://creature-lab-xyz.sprites.app") and
// returns a pack of its complete species. client may be nil.
func Pull(ctx context.Context, client *http.Client, base string) (Result, error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	api, err := apiRoot(base)
	if err != nil {
		return Result{}, err
	}
	var list speciesList
	if err := getJSON(ctx, client, api+"/species", &list); err != nil {
		return Result{}, err
	}
	res := Result{Pack: sim.SpeciesPack{
		Source:     strings.TrimSuffix(api, "/api"),
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Species:    []sim.PackedSpecies{},
	}}
	// The list is newest first; a pack reads better oldest first, and its
	// order is what a seed's draw is taken from, so keep it stable.
	for i := len(list.Species) - 1; i >= 0; i-- {
		item := list.Species[i]
		if !item.Complete {
			res.Incomplete++
			continue
		}
		var d speciesDetail
		if err := getJSON(ctx, client, api+"/species/"+url.PathEscape(item.ID), &d); err != nil {
			return Result{}, err
		}
		var c candidateList
		if err := getJSON(ctx, client, api+"/species/"+url.PathEscape(item.ID)+"/candidates?svg=1", &c); err != nil {
			return Result{}, err
		}
		ps, why := pack(d, c)
		if why != "" {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (%s, seed %d): %s", d.Traits.Plural, d.ID, d.Seed, why))
			continue
		}
		res.Pack.Species = append(res.Pack.Species, ps)
	}
	return res, nil
}

// pack rolls d's seed and checks it is the species the lab shows, then
// attaches each form's accepted sprite. why says what disagreed.
func pack(d speciesDetail, c candidateList) (ps sim.PackedSpecies, why string) {
	sp := sim.RollLabSpecies(d.Seed)
	t := d.Traits
	switch {
	case sp.Singular != t.Singular || sp.Plural != t.Plural:
		return ps, fmt.Sprintf("rolled by %s as %q; this build rolls %q from that seed", d.GeneratorRev, t.Plural, sp.Plural)
	case sp.ScientificName != t.ScientificName:
		return ps, fmt.Sprintf("rolled by %s as %s; this build rolls %s", d.GeneratorRev, t.ScientificName, sp.ScientificName)
	case sp.Temperament.String() != t.Temperament || sp.Apex != t.Apex:
		return ps, fmt.Sprintf("rolled by %s with a different temperament or apex than this build rolls", d.GeneratorRev)
	}
	names := formNames(sp)
	if len(names) != len(d.Forms) {
		return ps, fmt.Sprintf("rolled by %s with %d forms; this build rolls %d", d.GeneratorRev, len(d.Forms), len(names))
	}
	accepted := map[string]string{}
	for _, cand := range c.Candidates {
		if cand.Accepted {
			accepted[cand.ID] = cand.SVG
		}
	}
	ps = sim.PackedSpecies{ID: d.ID, Seed: d.Seed, GeneratorRev: d.GeneratorRev, Species: sp}
	for i, f := range d.Forms {
		if f.Index != i || f.Name != names[i] {
			return sim.PackedSpecies{}, fmt.Sprintf("rolled by %s with form %d a %s; this build rolls a %s", d.GeneratorRev, i, f.Name, names[i])
		}
		svg := accepted[f.AcceptedID]
		if f.AcceptedID == "" || svg == "" {
			return sim.PackedSpecies{}, fmt.Sprintf("form %d (%s) has no accepted sprite", i, f.Name)
		}
		ps.Sprites = append(ps.Sprites, sim.PackedSprite{Form: i, Name: f.Name, SVG: svg})
	}
	return ps, ""
}

// formNames are a species' forms as the lab names them: the stage or caste
// word, or "adult" for a plain adult and for a single-form species.
func formNames(sp sim.AlienSpecies) []string {
	life := sp.LifeForms()
	if len(life) == 0 {
		return []string{"adult"}
	}
	out := make([]string, len(life))
	for i, f := range life {
		out[i] = f.Name
		if out[i] == "" {
			out[i] = "adult"
		}
	}
	return out
}

// apiRoot is the lab's API base from a URL the person gave: the site
// ("https://lab.example"), or its API ("https://lab.example/api").
func apiRoot(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("lab URL %q: want http(s)://host", base)
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api")
	return u.Scheme + "://" + u.Host + path + "/api", nil
}

func getJSON(ctx context.Context, client *http.Client, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("GET %s: the lab wants a key for this; a pull uses only its public reads", u)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("GET %s: %s: %s", u, resp.Status, strings.TrimSpace(string(body[:min(len(body), 200)])))
	case len(body) > maxBody:
		return fmt.Errorf("GET %s: response over %d bytes", u, maxBody)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	return nil
}

// ErrNothing is returned by Encode for a pull with no species in it, so an
// unreachable or empty lab never overwrites a good pack with an empty one.
var ErrNothing = errors.New("no complete species to pack")

// Encode renders a pack as the file the game reads, checking it loads first.
func Encode(p sim.SpeciesPack) ([]byte, error) {
	if len(p.Species) == 0 {
		return nil, ErrNothing
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err := sim.LoadSpeciesPack(data, "pulled pack"); err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
