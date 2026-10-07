package creaturelab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Errors the surfaces (REST, MCP, web) map to their own responses.
var (
	ErrNotFound   = errors.New("not found")
	ErrNoSuchForm = errors.New("no such form")
	ErrBadInput   = errors.New("bad input")
)

// MaxPreview bounds one PreviewSpecies call.
const MaxPreview = 12

// Service is every Creature Lab operation. The REST API, the MCP tools and
// the web pages are thin presentations over it, so a rule (what a slot is,
// what an acceptable SVG is, what accepting does) lives in one place. It is
// the inventory app's Endpoint idea without the code generation: the
// surfaces here are small enough that one service with view-model returns
// carries it.
type Service struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
	// BaseURL is the lab's public origin, for the links in view models
	// ("https://creature-lab-xyz.sprites.app"). Empty gives relative links.
	BaseURL string
}

// NewService builds a Service on pool.
func NewService(pool *pgxpool.Pool, baseURL string) *Service {
	return &Service{Pool: pool, Q: db.New(pool), BaseURL: strings.TrimRight(baseURL, "/")}
}

// ---- view models ------------------------------------------------------------

// SpeciesPreview is a rolled species that is not saved.
type SpeciesPreview struct {
	Seed   int64  `json:"seed"`
	Traits Traits `json:"traits"`
	Forms  []Form `json:"forms"`
}

// SpeciesSummary is one row of the catalog.
type SpeciesSummary struct {
	ID             string    `json:"id"`
	Seed           int64     `json:"seed"`
	Singular       string    `json:"singular"`
	Plural         string    `json:"plural"`
	ScientificName string    `json:"scientificName"`
	Emoji          string    `json:"emoji,omitempty"`
	Temperament    string    `json:"temperament"`
	Apex           bool      `json:"apex,omitempty"`
	FormCount      int       `json:"formCount"`
	AcceptedCount  int       `json:"acceptedCount"`
	Complete       bool      `json:"complete"`
	CreatedAt      time.Time `json:"createdAt"`
	URL            string    `json:"url"`
	// SpriteURLs are the accepted sprites, in form order.
	SpriteURLs []string `json:"spriteUrls,omitempty"`
}

// SpeciesDetail is one species with its slots.
type SpeciesDetail struct {
	ID           string     `json:"id"`
	Seed         int64      `json:"seed"`
	GeneratorRev string     `json:"generatorRev"`
	Notes        string     `json:"notes,omitempty"`
	Traits       Traits     `json:"traits"`
	Forms        []FormView `json:"forms"`
	Complete     bool       `json:"complete"`
	CreatedAt    time.Time  `json:"createdAt"`
	URL          string     `json:"url"`
}

// FormView is a slot and where its sprite stands.
type FormView struct {
	Form
	AcceptedID     string `json:"acceptedId,omitempty"`
	AcceptedSVGURL string `json:"acceptedSvgUrl,omitempty"`
	Candidates     int    `json:"candidates"`
}

// Candidate is one drawing for a slot. SVG is left out of listings unless
// asked for.
type Candidate struct {
	ID        string    `json:"id"`
	SpeciesID string    `json:"speciesId"`
	Form      int       `json:"form"`
	SVG       string    `json:"svg,omitempty"`
	SVGURL    string    `json:"svgUrl"`
	Note      string    `json:"note,omitempty"`
	Author    string    `json:"author,omitempty"`
	Accepted  bool      `json:"accepted"`
	Warnings  []string  `json:"warnings,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// ---- species ----------------------------------------------------------------

// PreviewSpecies rolls count species without saving them: seed, seed+1, …
// from seed when it is non-zero, fresh seeds otherwise. Browsing is free, so
// a person (or Claude) can look at a handful and keep the one they like.
func (s *Service) PreviewSpecies(seed int64, count int) []SpeciesPreview {
	count = min(max(count, 1), MaxPreview)
	out := make([]SpeciesPreview, count)
	for i := range out {
		sd := seed + int64(i)
		if seed == 0 {
			sd = NewSeed()
		}
		sp := Roll(sd)
		out[i] = SpeciesPreview{Seed: sd, Traits: TraitsOf(sp), Forms: Forms(sp)}
	}
	return out
}

// CreateSpecies rolls the species for seed (a fresh one when seed is 0) and
// saves it.
func (s *Service) CreateSpecies(ctx context.Context, seed int64, notes string) (SpeciesDetail, error) {
	if seed == 0 {
		seed = NewSeed()
	}
	sp := Roll(seed)
	data, err := json.Marshal(sp)
	if err != nil {
		return SpeciesDetail{}, err
	}
	row, err := s.Q.CreateSpecies(ctx, db.CreateSpeciesParams{
		ID:             xid.New().String(),
		Seed:           seed,
		GeneratorRev:   GeneratorRev(),
		Singular:       sp.Singular,
		Plural:         sp.Plural,
		ScientificName: sp.ScientificName,
		Emoji:          sp.Emoji,
		Temperament:    sp.Temperament.String(),
		FormCount:      int32(len(Forms(sp))),
		Description:    sp.Description(),
		Data:           data,
		Notes:          strings.TrimSpace(notes),
		CreatedBy:      auth.KeyIDPtr(ctx),
	})
	if err != nil {
		return SpeciesDetail{}, err
	}
	return s.detail(row, nil)
}

// ListSpecies is the catalog, newest first.
func (s *Service) ListSpecies(ctx context.Context) ([]SpeciesSummary, error) {
	rows, err := s.Q.ListSpecies(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := s.Q.ListAllAccepted(ctx)
	if err != nil {
		return nil, err
	}
	sprites := map[string][]string{}
	for _, c := range acc { // ordered by species, then form
		sprites[c.SpeciesID] = append(sprites[c.SpeciesID], s.svgURL(c.ID))
	}
	out := make([]SpeciesSummary, len(rows))
	for i, r := range rows {
		// Flags with no column of their own come from the stored species.
		var flags struct{ Apex bool }
		_ = json.Unmarshal(r.Data, &flags)
		out[i] = SpeciesSummary{
			ID:             r.ID,
			Seed:           r.Seed,
			Singular:       r.Singular,
			Plural:         r.Plural,
			ScientificName: r.ScientificName,
			Emoji:          r.Emoji,
			Temperament:    r.Temperament,
			Apex:           flags.Apex,
			FormCount:      int(r.FormCount),
			AcceptedCount:  int(r.AcceptedCount),
			Complete:       int(r.AcceptedCount) >= int(r.FormCount),
			CreatedAt:      r.CreatedAt.Time,
			URL:            s.speciesURL(r.ID),
			SpriteURLs:     sprites[r.ID],
		}
	}
	return out, nil
}

// GetSpecies is one species with its slots.
func (s *Service) GetSpecies(ctx context.Context, id string) (SpeciesDetail, error) {
	row, err := s.Q.GetSpecies(ctx, id)
	if err != nil {
		return SpeciesDetail{}, notFound(err, "species", id)
	}
	cands, err := s.Q.ListCandidates(ctx, id)
	if err != nil {
		return SpeciesDetail{}, err
	}
	return s.detail(row, cands)
}

// SetSpeciesNotes replaces a species' free-text notes.
func (s *Service) SetSpeciesNotes(ctx context.Context, id, notes string) (SpeciesDetail, error) {
	if _, err := s.Q.UpdateSpeciesNotes(ctx, db.UpdateSpeciesNotesParams{ID: id, Notes: strings.TrimSpace(notes)}); err != nil {
		return SpeciesDetail{}, notFound(err, "species", id)
	}
	return s.GetSpecies(ctx, id)
}

// DeleteSpecies soft-deletes a species; its sprites go with it.
func (s *Service) DeleteSpecies(ctx context.Context, id string) error {
	n, err := s.Q.DeleteSpecies(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: species %s", ErrNotFound, id)
	}
	return nil
}

// species loads a row's stored sim.AlienSpecies.
func (s *Service) species(ctx context.Context, id string) (db.Species, sim.AlienSpecies, error) {
	row, err := s.Q.GetSpecies(ctx, id)
	if err != nil {
		return db.Species{}, sim.AlienSpecies{}, notFound(err, "species", id)
	}
	sp, err := decodeSpecies(row)
	return row, sp, err
}

func decodeSpecies(row db.Species) (sim.AlienSpecies, error) {
	var sp sim.AlienSpecies
	if err := json.Unmarshal(row.Data, &sp); err != nil {
		return sp, fmt.Errorf("species %s: stored data: %w", row.ID, err)
	}
	return sp, nil
}

func (s *Service) detail(row db.Species, cands []db.SpriteCandidate) (SpeciesDetail, error) {
	sp, err := decodeSpecies(row)
	if err != nil {
		return SpeciesDetail{}, err
	}
	forms := Forms(sp)
	views := make([]FormView, len(forms))
	accepted := 0
	for i, f := range forms {
		views[i].Form = f
	}
	for _, c := range cands {
		if int(c.Form) >= len(views) {
			continue
		}
		v := &views[c.Form]
		v.Candidates++
		if c.AcceptedAt.Valid {
			v.AcceptedID = c.ID
			v.AcceptedSVGURL = s.svgURL(c.ID)
			accepted++
		}
	}
	return SpeciesDetail{
		ID:           row.ID,
		Seed:         row.Seed,
		GeneratorRev: row.GeneratorRev,
		Notes:        row.Notes,
		Traits:       TraitsOf(sp),
		Forms:        views,
		Complete:     accepted >= len(forms),
		CreatedAt:    row.CreatedAt.Time,
		URL:          s.speciesURL(row.ID),
	}, nil
}

// ---- sprites ----------------------------------------------------------------

// SpriteBrief is the drawing brief for one slot, with the species' other
// accepted sprites attached so a life stays one creature.
func (s *Service) SpriteBrief(ctx context.Context, speciesID string, form int) (string, error) {
	_, sp, err := s.species(ctx, speciesID)
	if err != nil {
		return "", err
	}
	acc, err := s.Q.ListAcceptedForSpecies(ctx, speciesID)
	if err != nil {
		return "", err
	}
	siblings := make(map[int]string, len(acc))
	for _, c := range acc {
		siblings[int(c.Form)] = c.Svg
	}
	return Brief(sp, form, siblings)
}

// SubmitCandidate checks an SVG and stores it as a candidate for a slot.
// accept makes it the slot's sprite straight away.
func (s *Service) SubmitCandidate(ctx context.Context, speciesID string, form int, svg, note, author string, accept bool) (Candidate, error) {
	row, sp, err := s.species(ctx, speciesID)
	if err != nil {
		return Candidate{}, err
	}
	if n := len(Forms(sp)); form < 0 || form >= n {
		return Candidate{}, fmt.Errorf("%w: form %d (species %s has %d)", ErrNoSuchForm, form, row.ID, n)
	}
	clean, warnings, err := CheckSVG(svg)
	if err != nil {
		return Candidate{}, err
	}
	c, err := s.Q.CreateCandidate(ctx, db.CreateCandidateParams{
		ID:        xid.New().String(),
		SpeciesID: speciesID,
		Form:      int32(form),
		Svg:       clean,
		Note:      strings.TrimSpace(note),
		Author:    strings.TrimSpace(author),
		CreatedBy: auth.KeyIDPtr(ctx),
	})
	if err != nil {
		return Candidate{}, err
	}
	if accept {
		return s.AcceptCandidate(ctx, c.ID)
	}
	out := s.candidate(c, true)
	out.Warnings = warnings
	return out, nil
}

// ListCandidates is a species' candidates, newest first within each form;
// form < 0 lists every form. withSVG includes the source.
func (s *Service) ListCandidates(ctx context.Context, speciesID string, form int, withSVG bool) ([]Candidate, error) {
	if _, err := s.Q.GetSpecies(ctx, speciesID); err != nil {
		return nil, notFound(err, "species", speciesID)
	}
	rows, err := s.Q.ListCandidates(ctx, speciesID)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		if form >= 0 && int(r.Form) != form {
			continue
		}
		out = append(out, s.candidate(r, withSVG))
	}
	return out, nil
}

// GetCandidate is one candidate with its SVG and lint warnings.
func (s *Service) GetCandidate(ctx context.Context, id string) (Candidate, error) {
	r, err := s.Q.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, notFound(err, "candidate", id)
	}
	out := s.candidate(r, true)
	out.Warnings = Lint(r.Svg)
	return out, nil
}

// AcceptCandidate makes a candidate its slot's sprite, replacing whichever
// was accepted before (that one stays as a candidate).
func (s *Service) AcceptCandidate(ctx context.Context, id string) (Candidate, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Candidate{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit
	q := s.Q.WithTx(tx)
	c, err := q.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, notFound(err, "candidate", id)
	}
	if err := q.ClearAccepted(ctx, db.ClearAcceptedParams{SpeciesID: c.SpeciesID, Form: c.Form}); err != nil {
		return Candidate{}, err
	}
	if c, err = q.MarkAccepted(ctx, id); err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Candidate{}, err
	}
	return s.candidate(c, true), nil
}

// DeleteCandidate soft-deletes a candidate. Deleting the accepted one leaves
// its slot without a sprite.
func (s *Service) DeleteCandidate(ctx context.Context, id string) error {
	n, err := s.Q.DeleteCandidate(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: candidate %s", ErrNotFound, id)
	}
	return nil
}

// CandidateSVG is the raw SVG, for serving as an image.
func (s *Service) CandidateSVG(ctx context.Context, id string) (string, error) {
	r, err := s.Q.GetCandidate(ctx, id)
	if err != nil {
		return "", notFound(err, "candidate", id)
	}
	return r.Svg, nil
}

func (s *Service) candidate(r db.SpriteCandidate, withSVG bool) Candidate {
	c := Candidate{
		ID:        r.ID,
		SpeciesID: r.SpeciesID,
		Form:      int(r.Form),
		SVGURL:    s.svgURL(r.ID),
		Note:      r.Note,
		Author:    r.Author,
		Accepted:  r.AcceptedAt.Valid,
		CreatedAt: r.CreatedAt.Time,
	}
	if withSVG {
		c.SVG = r.Svg
	}
	return c
}

// ---- export -----------------------------------------------------------------

// Export is the catalog in the form a game build would load: each species as
// rolled (the sim.AlienSpecies JSON, unchanged) with its accepted sprites.
type Export struct {
	ExportedAt time.Time       `json:"exportedAt"`
	Species    []ExportSpecies `json:"species"`
}

// ExportSpecies is one species in an Export.
type ExportSpecies struct {
	ID           string          `json:"id"`
	Seed         int64           `json:"seed"`
	GeneratorRev string          `json:"generatorRev"`
	Complete     bool            `json:"complete"`
	Species      json.RawMessage `json:"species"`
	Sprites      []ExportSprite  `json:"sprites"`
}

// ExportSprite is one accepted sprite.
type ExportSprite struct {
	Form int    `json:"form"`
	Name string `json:"name"`
	SVG  string `json:"svg"`
}

// Export lists species that have a sprite for every form, or every species
// when includeIncomplete is set.
func (s *Service) Export(ctx context.Context, includeIncomplete bool) (Export, error) {
	rows, err := s.Q.ListSpecies(ctx)
	if err != nil {
		return Export{}, err
	}
	acc, err := s.Q.ListAllAccepted(ctx)
	if err != nil {
		return Export{}, err
	}
	sprites := map[string][]db.SpriteCandidate{}
	for _, c := range acc {
		sprites[c.SpeciesID] = append(sprites[c.SpeciesID], c)
	}
	out := Export{ExportedAt: time.Now().UTC(), Species: []ExportSpecies{}}
	for _, r := range rows {
		complete := int(r.AcceptedCount) >= int(r.FormCount)
		if !complete && !includeIncomplete {
			continue
		}
		sp, err := decodeSpecies(db.Species{ID: r.ID, Data: r.Data})
		if err != nil {
			return Export{}, err
		}
		forms := Forms(sp)
		es := ExportSpecies{ID: r.ID, Seed: r.Seed, GeneratorRev: r.GeneratorRev, Complete: complete, Species: r.Data, Sprites: []ExportSprite{}}
		for _, c := range sprites[r.ID] {
			name := ""
			if int(c.Form) < len(forms) {
				name = forms[c.Form].Name
			}
			es.Sprites = append(es.Sprites, ExportSprite{Form: int(c.Form), Name: name, SVG: c.Svg})
		}
		out.Species = append(out.Species, es)
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------------

func (s *Service) speciesURL(id string) string { return s.BaseURL + "/species/" + id }
func (s *Service) svgURL(id string) string     { return s.BaseURL + "/sprites/" + id + ".svg" }

// notFound turns pgx's no-rows into ErrNotFound.
func notFound(err error, what, id string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s %s", ErrNotFound, what, id)
	}
	return err
}
