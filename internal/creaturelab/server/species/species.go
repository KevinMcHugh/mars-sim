// Package species holds the endpoints for the catalog's species: preview,
// list, get, create, update and delete.
package species

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	apigen "github.com/kevinmchugh/mars-sim/internal/creaturelab/api/gen"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// -----------------------------------------------------------------------------
// View models
// -----------------------------------------------------------------------------

// Preview is a rolled species that is not saved.
type Preview struct {
	Seed   int64              `json:"seed"`
	Traits creaturelab.Traits `json:"traits"`
	Forms  []creaturelab.Form `json:"forms"`
}

// Summary is one row of the catalog.
type Summary struct {
	ID             string    `json:"id"`
	Seed           int64     `json:"seed"`
	Singular       string    `json:"singular"`
	Plural         string    `json:"plural"`
	NameEdited     bool      `json:"nameEdited,omitempty"`
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

// Detail is one species with its slots. Traits.Description is the field
// notes as shown, rewritten or generated, and Traits.Singular and Plural are
// the common name as shown, renamed or rolled.
type Detail struct {
	ID           string             `json:"id"`
	Seed         int64              `json:"seed"`
	GeneratorRev string             `json:"generatorRev"`
	Notes        string             `json:"notes,omitempty"`
	Traits       creaturelab.Traits `json:"traits"`
	// GeneratedDescription is the roster code's text, as rolled.
	GeneratedDescription string `json:"generatedDescription"`
	DescriptionEdited    bool   `json:"descriptionEdited,omitempty"`
	// GeneratedSingular and GeneratedPlural are the name it rolled with.
	GeneratedSingular string     `json:"generatedSingular"`
	GeneratedPlural   string     `json:"generatedPlural"`
	NameEdited        bool       `json:"nameEdited,omitempty"`
	Forms             []FormView `json:"forms"`
	Complete          bool       `json:"complete"`
	CreatedAt         time.Time  `json:"createdAt"`
	URL               string     `json:"url"`
}

// FormView is a slot and where its sprite stands.
type FormView struct {
	creaturelab.Form
	AcceptedID     string `json:"acceptedId,omitempty"`
	AcceptedSVGURL string `json:"acceptedSvgUrl,omitempty"`
	Candidates     int    `json:"candidates"`
}

// Model is a stored species, decoded, with its live candidates (nil when
// they were not loaded).
type Model struct {
	Row        db.Species
	Species    sim.AlienSpecies
	Candidates []db.SpriteCandidate
}

// ToDetail is the view of m.
func ToDetail(m Model, links creaturelab.Links) Detail {
	forms := creaturelab.Forms(m.Species)
	views := make([]FormView, len(forms))
	for i, f := range forms {
		views[i].Form = f
	}
	accepted := 0
	for _, c := range m.Candidates {
		if int(c.Form) >= len(views) {
			continue
		}
		v := &views[c.Form]
		v.Candidates++
		if c.AcceptedAt.Valid {
			v.AcceptedID = c.ID
			v.AcceptedSVGURL = links.Sprite(c.ID)
			accepted++
		}
	}
	traits := creaturelab.TraitsOf(m.Species)
	traits.Description = creaturelab.FieldNotes(m.Row)
	return Detail{
		ID:                   m.Row.ID,
		Seed:                 m.Row.Seed,
		GeneratorRev:         m.Row.GeneratorRev,
		Notes:                m.Row.Notes,
		Traits:               traits,
		GeneratedDescription: m.Row.Description,
		DescriptionEdited:    m.Row.DescriptionOverride != "",
		GeneratedSingular:    m.Row.Singular,
		GeneratedPlural:      m.Row.Plural,
		NameEdited:           creaturelab.NameEdited(m.Row),
		Forms:                views,
		Complete:             accepted >= len(forms),
		CreatedAt:            m.Row.CreatedAt.Time,
		URL:                  links.Species(m.Row.ID),
	}
}

// load is a species and its candidates, or ErrNotFound.
func load(ctx context.Context, store GetStore, id string) (Model, error) {
	row, err := store.GetSpecies(ctx, id)
	if err != nil {
		return Model{}, creaturelab.NotFound(err, "species", id)
	}
	return withCandidates(ctx, store, row)
}

// CandidateLister is the store method that loads a species' candidates.
type CandidateLister interface {
	ListCandidates(ctx context.Context, speciesID string) ([]db.SpriteCandidate, error)
}

func withCandidates(ctx context.Context, store CandidateLister, row db.Species) (Model, error) {
	sp, err := creaturelab.DecodeSpecies(row)
	if err != nil {
		return Model{}, err
	}
	cands, err := store.ListCandidates(ctx, row.ID)
	if err != nil {
		return Model{}, err
	}
	return Model{Row: row, Species: sp, Candidates: cands}, nil
}

// -----------------------------------------------------------------------------
// API conversion
// -----------------------------------------------------------------------------

// TraitsToAPI is t as the API type.
func TraitsToAPI(t creaturelab.Traits) apigen.Traits {
	return apigen.Traits{
		Singular:       t.Singular,
		Plural:         t.Plural,
		ScientificName: t.ScientificName,
		Emoji:          t.Emoji,
		Temperament:    t.Temperament,
		Apex:           t.Apex,
		Skin:           t.Skin,
		Color:          t.Color,
		Pattern:        t.Pattern,
		Eyes:           t.Eyes,
		Arms:           t.Arms,
		Legs:           t.Legs,
		Tail:           t.Tail,
		Wings:          t.Wings,
		HeightCM:       t.HeightCM[:],
		WeightKG:       t.WeightKG[:],
		Attacks:        t.Attacks,
		Features:       t.Features,
		Description:    t.Description,
	}
}

// FormToAPI is f as the API type.
func FormToAPI(f creaturelab.Form) apigen.Form {
	return apigen.Form{
		Index:    f.Index,
		Name:     f.Name,
		Label:    f.Label,
		Stage:    f.Stage,
		SizePct:  f.SizePct,
		Inert:    f.Inert,
		Arms:     f.Arms,
		Legs:     f.Legs,
		Tail:     f.Tail,
		Wings:    f.Wings,
		Features: f.Features,
		Lays:     f.Lays,
	}
}

func formsToAPI(fs []creaturelab.Form) []apigen.Form {
	out := make([]apigen.Form, len(fs))
	for i, f := range fs {
		out[i] = FormToAPI(f)
	}
	return out
}

func detailToAPI(d Detail) apigen.SpeciesDetail {
	forms := make([]apigen.FormView, len(d.Forms))
	for i, f := range d.Forms {
		forms[i] = apigen.FormView{
			Index:          f.Index,
			Name:           f.Name,
			Label:          f.Label,
			Stage:          f.Stage,
			SizePct:        f.SizePct,
			Inert:          f.Inert,
			Arms:           f.Arms,
			Legs:           f.Legs,
			Tail:           f.Tail,
			Wings:          f.Wings,
			Features:       f.Features,
			Lays:           f.Lays,
			AcceptedId:     f.AcceptedID,
			AcceptedSvgUrl: f.AcceptedSVGURL,
			Candidates:     f.Candidates,
		}
	}
	return apigen.SpeciesDetail{
		Id:                   d.ID,
		Seed:                 d.Seed,
		GeneratorRev:         d.GeneratorRev,
		Notes:                d.Notes,
		Traits:               TraitsToAPI(d.Traits),
		GeneratedDescription: d.GeneratedDescription,
		DescriptionEdited:    d.DescriptionEdited,
		GeneratedSingular:    d.GeneratedSingular,
		GeneratedPlural:      d.GeneratedPlural,
		NameEdited:           d.NameEdited,
		Forms:                forms,
		Complete:             d.Complete,
		CreatedAt:            d.CreatedAt,
		Url:                  d.URL,
	}
}

func summaryToAPI(s Summary) apigen.SpeciesSummary {
	return apigen.SpeciesSummary{
		Id:             s.ID,
		Seed:           s.Seed,
		Singular:       s.Singular,
		Plural:         s.Plural,
		NameEdited:     s.NameEdited,
		ScientificName: s.ScientificName,
		Emoji:          s.Emoji,
		Temperament:    s.Temperament,
		Apex:           s.Apex,
		FormCount:      s.FormCount,
		AcceptedCount:  s.AcceptedCount,
		Complete:       s.Complete,
		CreatedAt:      s.CreatedAt,
		Url:            s.URL,
		SpriteUrls:     s.SpriteURLs,
	}
}

// -----------------------------------------------------------------------------
// Preview species
// -----------------------------------------------------------------------------

// PreviewEndpoint rolls species without saving them: seed, seed+1, … from
// seed when it is non-zero, fresh seeds otherwise. Browsing is free, so a
// person (or Claude) can look at a handful and keep the one they like. It
// touches no store.
type PreviewEndpoint struct{}

type rolled struct {
	Seed    int64
	Species sim.AlienSpecies
}

func (PreviewEndpoint) Interact(_ context.Context, req apigen.PreviewSpeciesRequestObject) ([]rolled, error) {
	count := req.Params.Count
	if count == 0 {
		count = 3
	}
	count = min(max(count, 1), creaturelab.MaxPreview)
	seed := req.Params.Seed
	out := make([]rolled, count)
	for i := range out {
		sd := seed + int64(i)
		if seed == 0 {
			sd = creaturelab.NewSeed()
		}
		out[i] = rolled{Seed: sd, Species: creaturelab.Roll(sd)}
	}
	return out, nil
}

func (PreviewEndpoint) Build(rs []rolled) []Preview {
	out := make([]Preview, len(rs))
	for i, r := range rs {
		out[i] = Preview{Seed: r.Seed, Traits: creaturelab.TraitsOf(r.Species), Forms: creaturelab.Forms(r.Species)}
	}
	return out
}

func (PreviewEndpoint) Render(vms []Preview) apigen.PreviewSpeciesResponseObject {
	out := make([]apigen.SpeciesPreview, len(vms))
	for i, p := range vms {
		out[i] = apigen.SpeciesPreview{Seed: p.Seed, Traits: TraitsToAPI(p.Traits), Forms: formsToAPI(p.Forms)}
	}
	return apigen.PreviewSpecies200JSONResponse{Species: out}
}

// -----------------------------------------------------------------------------
// List species
// -----------------------------------------------------------------------------

type ListStore interface {
	ListSpecies(ctx context.Context) ([]db.ListSpeciesRow, error)
	ListAllAccepted(ctx context.Context) ([]db.SpriteCandidate, error)
}

// ListEndpoint is the catalog, newest first. It is public.
type ListEndpoint struct {
	Store ListStore
	Links creaturelab.Links
}

type listModel struct {
	Rows     []db.ListSpeciesRow
	Accepted []db.SpriteCandidate // ordered by species, then form
}

func (e ListEndpoint) Interact(ctx context.Context, _ apigen.ListSpeciesRequestObject) (listModel, error) {
	rows, err := e.Store.ListSpecies(ctx)
	if err != nil {
		return listModel{}, err
	}
	acc, err := e.Store.ListAllAccepted(ctx)
	if err != nil {
		return listModel{}, err
	}
	return listModel{Rows: rows, Accepted: acc}, nil
}

func (e ListEndpoint) Build(m listModel) []Summary {
	sprites := map[string][]string{}
	for _, c := range m.Accepted {
		sprites[c.SpeciesID] = append(sprites[c.SpeciesID], e.Links.Sprite(c.ID))
	}
	out := make([]Summary, len(m.Rows))
	for i, r := range m.Rows {
		// Flags with no column of their own come from the stored species.
		var flags struct{ Apex bool }
		_ = json.Unmarshal(r.Data, &flags)
		row := db.Species{Singular: r.Singular, Plural: r.Plural, SingularOverride: r.SingularOverride, PluralOverride: r.PluralOverride}
		singular, plural := creaturelab.CommonName(row)
		out[i] = Summary{
			ID:             r.ID,
			Seed:           r.Seed,
			Singular:       singular,
			Plural:         plural,
			NameEdited:     creaturelab.NameEdited(row),
			ScientificName: r.ScientificName,
			Emoji:          r.Emoji,
			Temperament:    r.Temperament,
			Apex:           flags.Apex,
			FormCount:      int(r.FormCount),
			AcceptedCount:  int(r.AcceptedCount),
			Complete:       int(r.AcceptedCount) >= int(r.FormCount),
			CreatedAt:      r.CreatedAt.Time,
			URL:            e.Links.Species(r.ID),
			SpriteURLs:     sprites[r.ID],
		}
	}
	return out
}

func (e ListEndpoint) Render(vms []Summary) apigen.ListSpeciesResponseObject {
	out := make([]apigen.SpeciesSummary, len(vms))
	for i, s := range vms {
		out[i] = summaryToAPI(s)
	}
	return apigen.ListSpecies200JSONResponse{Species: out}
}

// -----------------------------------------------------------------------------
// Get species
// -----------------------------------------------------------------------------

type GetStore interface {
	GetSpecies(ctx context.Context, id string) (db.Species, error)
	CandidateLister
}

// GetEndpoint is one species with its slots. It is public.
type GetEndpoint struct {
	Store GetStore
	Links creaturelab.Links
}

func (e GetEndpoint) Interact(ctx context.Context, req apigen.GetSpeciesRequestObject) (Model, error) {
	return load(ctx, e.Store, req.SpeciesId)
}

func (e GetEndpoint) Build(m Model) Detail { return ToDetail(m, e.Links) }

func (e GetEndpoint) Render(vm Detail) apigen.GetSpeciesResponseObject {
	return apigen.GetSpecies200JSONResponse(detailToAPI(vm))
}

// -----------------------------------------------------------------------------
// Create species
// -----------------------------------------------------------------------------

type CreateStore interface {
	CreateSpecies(ctx context.Context, arg db.CreateSpeciesParams) (db.Species, error)
}

// CreateEndpoint rolls the species for a seed (a fresh one when it is 0)
// and saves it, recording the commit that rolled it.
type CreateEndpoint struct {
	Store CreateStore
	Links creaturelab.Links
}

func (e CreateEndpoint) Interact(ctx context.Context, req apigen.CreateSpeciesRequestObject) (Model, error) {
	seed := req.Body.Seed
	if seed == 0 {
		seed = creaturelab.NewSeed()
	}
	sp := creaturelab.Roll(seed)
	data, err := json.Marshal(sp)
	if err != nil {
		return Model{}, err
	}
	row, err := e.Store.CreateSpecies(ctx, db.CreateSpeciesParams{
		ID:             xid.New().String(),
		Seed:           seed,
		GeneratorRev:   creaturelab.GeneratorRev(),
		Singular:       sp.Singular,
		Plural:         sp.Plural,
		ScientificName: sp.ScientificName,
		Emoji:          sp.Emoji,
		Temperament:    sp.Temperament.String(),
		FormCount:      int32(len(creaturelab.Forms(sp))),
		Description:    sp.Description(),
		Data:           data,
		Notes:          strings.TrimSpace(req.Body.Notes),
		CreatedBy:      auth.KeyIDPtr(ctx),
	})
	if err != nil {
		return Model{}, err
	}
	return Model{Row: row, Species: sp}, nil
}

func (e CreateEndpoint) Build(m Model) Detail { return ToDetail(m, e.Links) }

func (e CreateEndpoint) Render(vm Detail) apigen.CreateSpeciesResponseObject {
	return apigen.CreateSpecies201JSONResponse(detailToAPI(vm))
}

// -----------------------------------------------------------------------------
// Update species
// -----------------------------------------------------------------------------

type UpdateStore interface {
	UpdateSpecies(ctx context.Context, arg db.UpdateSpeciesParams) (db.Species, error)
	GetStore
}

// UpdateEndpoint replaces a species' lab notes, its field notes, its common
// name, or any of them; a field left nil is left alone. Empty field notes go
// back to the generated text, and an empty name to the rolled one. Rewriting
// them is how an agent gives a species a voice, or a name, the roster's
// template does not; what was rolled is kept regardless.
type UpdateEndpoint struct {
	Store UpdateStore
	Links creaturelab.Links
}

func (e UpdateEndpoint) Interact(ctx context.Context, req apigen.UpdateSpeciesRequestObject) (Model, error) {
	arg := db.UpdateSpeciesParams{ID: req.SpeciesId}
	if n := req.Body.Notes; n != nil {
		trimmed := strings.TrimSpace(*n)
		arg.Notes = &trimmed
	}
	if d := req.Body.Description; d != nil {
		trimmed := strings.TrimSpace(*d)
		if n := utf8.RuneCountInString(trimmed); n > creaturelab.MaxDescription {
			return Model{}, fmt.Errorf("%w: description is %d characters; the limit is %d", creaturelab.ErrBadInput, n, creaturelab.MaxDescription)
		}
		arg.DescriptionOverride = &trimmed
	}
	if req.Body.Singular != nil || req.Body.Plural != "" {
		current, err := e.Store.GetSpecies(ctx, req.SpeciesId)
		if err != nil {
			return Model{}, creaturelab.NotFound(err, "species", req.SpeciesId)
		}
		singular, plural, err := rename(current, req.Body.Singular, req.Body.Plural)
		if err != nil {
			return Model{}, err
		}
		arg.SingularOverride, arg.PluralOverride = &singular, &plural
	}
	row, err := e.Store.UpdateSpecies(ctx, arg)
	if err != nil {
		return Model{}, creaturelab.NotFound(err, "species", req.SpeciesId)
	}
	return withCandidates(ctx, e.Store, row)
}

// rename checks a new common name for row and returns the overrides to
// store: both empty to go back to the rolled name, which is also what a
// "rename" to the rolled name itself stores. An empty plural is the
// singular's plural as the game would make it.
func rename(row db.Species, singular *string, plural string) (string, string, error) {
	if singular == nil {
		return "", "", fmt.Errorf("%w: a plural needs its singular", creaturelab.ErrBadInput)
	}
	s, p := cleanName(*singular), cleanName(plural)
	if s == "" {
		return "", "", nil
	}
	if p == "" {
		p = sim.PluralName(s)
	}
	for _, n := range []string{s, p} {
		if c := utf8.RuneCountInString(n); c > creaturelab.MaxName {
			return "", "", fmt.Errorf("%w: name %q is %d characters; the limit is %d", creaturelab.ErrBadInput, n, c, creaturelab.MaxName)
		}
		if strings.IndexFunc(n, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
			return "", "", fmt.Errorf("%w: name %q has a character that does not print", creaturelab.ErrBadInput, n)
		}
	}
	if s == row.Singular && p == row.Plural {
		return "", "", nil
	}
	return s, p, nil
}

// cleanName trims a name and squeezes its inner runs of space to one.
func cleanName(s string) string { return strings.Join(strings.Fields(s), " ") }

func (e UpdateEndpoint) Build(m Model) Detail { return ToDetail(m, e.Links) }

func (e UpdateEndpoint) Render(vm Detail) apigen.UpdateSpeciesResponseObject {
	return apigen.UpdateSpecies200JSONResponse(detailToAPI(vm))
}

// -----------------------------------------------------------------------------
// Delete species
// -----------------------------------------------------------------------------

type DeleteStore interface {
	DeleteSpecies(ctx context.Context, id string) (int64, error)
}

// DeleteEndpoint soft-deletes a species; its sprites go with it.
type DeleteEndpoint struct{ Store DeleteStore }

func (e DeleteEndpoint) Interact(ctx context.Context, req apigen.DeleteSpeciesRequestObject) (bool, error) {
	n, err := e.Store.DeleteSpecies(ctx, req.SpeciesId)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, fmt.Errorf("%w: species %s", creaturelab.ErrNotFound, req.SpeciesId)
	}
	return true, nil
}

func (DeleteEndpoint) Build(ok bool) bool { return ok }

func (DeleteEndpoint) Render(ok bool) apigen.DeleteSpeciesResponseObject {
	return apigen.DeleteSpecies200JSONResponse{Ok: ok}
}
