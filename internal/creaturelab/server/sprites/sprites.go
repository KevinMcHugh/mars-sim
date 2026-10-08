// Package sprites holds the endpoints for drawing a species: a form's brief,
// and the candidate SVGs submitted for it.
package sprites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	apigen "github.com/kevinmchugh/mars-sim/internal/creaturelab/api/gen"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
)

// -----------------------------------------------------------------------------
// Candidate view model
// -----------------------------------------------------------------------------

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

// ToViewModel is the view of r; withSVG includes the source.
func ToViewModel(r db.SpriteCandidate, withSVG bool, links creaturelab.Links) Candidate {
	c := Candidate{
		ID:        r.ID,
		SpeciesID: r.SpeciesID,
		Form:      int(r.Form),
		SVGURL:    links.Sprite(r.ID),
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

func toAPI(c Candidate) apigen.Candidate {
	return apigen.Candidate{
		Id:        c.ID,
		SpeciesId: c.SpeciesID,
		Form:      c.Form,
		Svg:       c.SVG,
		SvgUrl:    c.SVGURL,
		Note:      c.Note,
		Author:    c.Author,
		Accepted:  c.Accepted,
		Warnings:  c.Warnings,
		CreatedAt: c.CreatedAt,
	}
}

// -----------------------------------------------------------------------------
// Get sprite brief
// -----------------------------------------------------------------------------

type BriefStore interface {
	GetSpecies(ctx context.Context, id string) (db.Species, error)
	ListAcceptedForSpecies(ctx context.Context, speciesID string) ([]db.SpriteCandidate, error)
}

// BriefEndpoint is the drawing brief for one slot, with the species' other
// accepted sprites attached so a life stays one creature.
type BriefEndpoint struct{ Store BriefStore }

// Brief is a slot and what an artist draws it from.
type Brief struct {
	Form  creaturelab.Form `json:"form"`
	Brief string           `json:"brief"`
}

func (e BriefEndpoint) Interact(ctx context.Context, req apigen.GetSpriteBriefRequestObject) (Brief, error) {
	row, err := e.Store.GetSpecies(ctx, req.SpeciesId)
	if err != nil {
		return Brief{}, creaturelab.NotFound(err, "species", req.SpeciesId)
	}
	sp, err := creaturelab.DecodeSpecies(row)
	if err != nil {
		return Brief{}, err
	}
	acc, err := e.Store.ListAcceptedForSpecies(ctx, row.ID)
	if err != nil {
		return Brief{}, err
	}
	siblings := make(map[int]string, len(acc))
	for _, c := range acc {
		siblings[int(c.Form)] = c.Svg
	}
	text, err := creaturelab.Brief(sp, creaturelab.FieldNotes(row), req.Form, siblings)
	if err != nil {
		return Brief{}, err
	}
	return Brief{Form: creaturelab.Forms(sp)[req.Form], Brief: text}, nil
}

func (BriefEndpoint) Build(b Brief) Brief { return b }

func (BriefEndpoint) Render(b Brief) apigen.GetSpriteBriefResponseObject {
	return apigen.GetSpriteBrief200JSONResponse{Form: b.Form.Index, Brief: b.Brief}
}

// -----------------------------------------------------------------------------
// List sprite candidates
// -----------------------------------------------------------------------------

type ListStore interface {
	GetSpecies(ctx context.Context, id string) (db.Species, error)
	ListCandidates(ctx context.Context, speciesID string) ([]db.SpriteCandidate, error)
}

// ListEndpoint is a species' candidates, newest first within each form.
type ListEndpoint struct {
	Store ListStore
	Links creaturelab.Links
}

type listModel struct {
	Rows    []db.SpriteCandidate
	WithSVG bool
}

func (e ListEndpoint) Interact(ctx context.Context, req apigen.ListSpriteCandidatesRequestObject) (listModel, error) {
	if _, err := e.Store.GetSpecies(ctx, req.SpeciesId); err != nil {
		return listModel{}, creaturelab.NotFound(err, "species", req.SpeciesId)
	}
	rows, err := e.Store.ListCandidates(ctx, req.SpeciesId)
	if err != nil {
		return listModel{}, err
	}
	if f := req.Params.Form; f != nil {
		kept := rows[:0]
		for _, r := range rows {
			if int(r.Form) == *f {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	return listModel{Rows: rows, WithSVG: req.Params.Svg}, nil
}

func (e ListEndpoint) Build(m listModel) []Candidate {
	out := make([]Candidate, len(m.Rows))
	for i, r := range m.Rows {
		out[i] = ToViewModel(r, m.WithSVG, e.Links)
	}
	return out
}

func (ListEndpoint) Render(vms []Candidate) apigen.ListSpriteCandidatesResponseObject {
	out := make([]apigen.Candidate, len(vms))
	for i, c := range vms {
		out[i] = toAPI(c)
	}
	return apigen.ListSpriteCandidates200JSONResponse{Candidates: out}
}

// -----------------------------------------------------------------------------
// Get sprite candidate
// -----------------------------------------------------------------------------

type GetStore interface {
	GetCandidate(ctx context.Context, id string) (db.SpriteCandidate, error)
}

// GetEndpoint is one candidate with its SVG and house-style warnings.
type GetEndpoint struct {
	Store GetStore
	Links creaturelab.Links
}

func (e GetEndpoint) Interact(ctx context.Context, req apigen.GetSpriteCandidateRequestObject) (db.SpriteCandidate, error) {
	r, err := e.Store.GetCandidate(ctx, req.CandidateId)
	return r, creaturelab.NotFound(err, "candidate", req.CandidateId)
}

func (e GetEndpoint) Build(r db.SpriteCandidate) Candidate {
	c := ToViewModel(r, true, e.Links)
	c.Warnings = creaturelab.Lint(r.Svg)
	return c
}

func (GetEndpoint) Render(c Candidate) apigen.GetSpriteCandidateResponseObject {
	return apigen.GetSpriteCandidate200JSONResponse(toAPI(c))
}

// -----------------------------------------------------------------------------
// Submit sprite candidate
// -----------------------------------------------------------------------------

type SubmitStore interface {
	GetSpecies(ctx context.Context, id string) (db.Species, error)
	CreateCandidate(ctx context.Context, arg db.CreateCandidateParams) (db.SpriteCandidate, error)
	AcceptCandidate(ctx context.Context, id string) (db.SpriteCandidate, error)
}

// SubmitEndpoint checks an SVG and stores it as a candidate for a slot.
// Accept makes it the slot's sprite straight away.
type SubmitEndpoint struct {
	Store SubmitStore
	Links creaturelab.Links
}

type submitted struct {
	Row      db.SpriteCandidate
	Warnings []string
}

func (e SubmitEndpoint) Interact(ctx context.Context, req apigen.SubmitSpriteCandidateRequestObject) (submitted, error) {
	row, err := e.Store.GetSpecies(ctx, req.SpeciesId)
	if err != nil {
		return submitted{}, creaturelab.NotFound(err, "species", req.SpeciesId)
	}
	sp, err := creaturelab.DecodeSpecies(row)
	if err != nil {
		return submitted{}, err
	}
	if n := len(creaturelab.Forms(sp)); req.Form < 0 || req.Form >= n {
		return submitted{}, fmt.Errorf("%w: form %d (species %s has %d)", creaturelab.ErrNoSuchForm, req.Form, row.ID, n)
	}
	clean, warnings, err := creaturelab.CheckSVG(req.Body.Svg)
	if err != nil {
		return submitted{}, err
	}
	c, err := e.Store.CreateCandidate(ctx, db.CreateCandidateParams{
		ID:        xid.New().String(),
		SpeciesID: row.ID,
		Form:      int32(req.Form),
		Svg:       clean,
		Note:      strings.TrimSpace(req.Body.Note),
		Author:    strings.TrimSpace(req.Body.Author),
		CreatedBy: auth.KeyIDPtr(ctx),
	})
	if err != nil {
		return submitted{}, err
	}
	if req.Body.Accept {
		if c, err = e.Store.AcceptCandidate(ctx, c.ID); err != nil {
			return submitted{}, err
		}
	}
	return submitted{Row: c, Warnings: warnings}, nil
}

func (e SubmitEndpoint) Build(s submitted) Candidate {
	c := ToViewModel(s.Row, true, e.Links)
	c.Warnings = s.Warnings
	return c
}

func (SubmitEndpoint) Render(c Candidate) apigen.SubmitSpriteCandidateResponseObject {
	return apigen.SubmitSpriteCandidate201JSONResponse(toAPI(c))
}

// -----------------------------------------------------------------------------
// Accept sprite candidate
// -----------------------------------------------------------------------------

type AcceptStore interface {
	AcceptCandidate(ctx context.Context, id string) (db.SpriteCandidate, error)
}

// AcceptEndpoint makes a candidate its slot's sprite, replacing whichever
// was accepted before (that one stays as a candidate).
type AcceptEndpoint struct {
	Store AcceptStore
	Links creaturelab.Links
}

func (e AcceptEndpoint) Interact(ctx context.Context, req apigen.AcceptSpriteCandidateRequestObject) (db.SpriteCandidate, error) {
	return e.Store.AcceptCandidate(ctx, req.CandidateId)
}

func (e AcceptEndpoint) Build(r db.SpriteCandidate) Candidate { return ToViewModel(r, true, e.Links) }

func (AcceptEndpoint) Render(c Candidate) apigen.AcceptSpriteCandidateResponseObject {
	return apigen.AcceptSpriteCandidate200JSONResponse(toAPI(c))
}

// -----------------------------------------------------------------------------
// Delete sprite candidate
// -----------------------------------------------------------------------------

type DeleteStore interface {
	DeleteCandidate(ctx context.Context, id string) (int64, error)
}

// DeleteEndpoint soft-deletes a candidate. Deleting the accepted one leaves
// its slot without a sprite.
type DeleteEndpoint struct{ Store DeleteStore }

func (e DeleteEndpoint) Interact(ctx context.Context, req apigen.DeleteSpriteCandidateRequestObject) (bool, error) {
	n, err := e.Store.DeleteCandidate(ctx, req.CandidateId)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, fmt.Errorf("%w: candidate %s", creaturelab.ErrNotFound, req.CandidateId)
	}
	return true, nil
}

func (DeleteEndpoint) Build(ok bool) bool { return ok }

func (DeleteEndpoint) Render(ok bool) apigen.DeleteSpriteCandidateResponseObject {
	return apigen.DeleteSpriteCandidate200JSONResponse{Ok: ok}
}
