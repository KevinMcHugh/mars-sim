// Package labmcp exposes Creature Lab over the Model Context Protocol, so a
// person can build the species catalog by talking to Claude: roll and keep
// species, read the brief for a form, draw it, submit the SVG, and accept the
// one they like. Every tool is a thin wrapper over creaturelab.Service, the
// same calls the REST API and web pages make.
//
// The server never calls a model itself. The connected client draws, so a
// sprite costs whatever the person's own Claude plan charges, when they
// choose to spend it.
package labmcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
)

// Instructions is sent to every client on connect.
const Instructions = `Creature Lab is the catalog of alien species for mars-sim, each with an SVG map sprite for every form of its life (egg, grub, adult, queen, ...; most species have just one form).

Typical session:
1. preview_species to roll a few unsaved species and pick one the user likes (free; nothing is stored).
2. create_species with that seed to save it.
3. For each form that has no accepted sprite: get_sprite_brief, draw an SVG that follows it exactly, and submit_sprite_candidate. Show the user the svgUrl or the SVG itself, revise and resubmit as they ask (each submission is a new candidate), then accept_sprite_candidate on the one they approve.
Forms are numbered from 0 in stage order. A species is complete when every form has an accepted sprite. Do not accept a sprite the user has not approved unless they asked you to.`

// NewServer builds the MCP server with every tool registered.
func NewServer(svc *creaturelab.Service) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "creature-lab", Version: "0.1.0"},
		&mcpsdk.ServerOptions{Instructions: Instructions})
	registerRead(s, svc)
	registerWrite(s, svc)
	return s
}

// ---- inputs and outputs -----------------------------------------------------

type PreviewInput struct {
	Seed  int64 `json:"seed,omitempty" jsonschema:"first seed to roll (then seed+1, ...); omit for random seeds"`
	Count int   `json:"count,omitempty" jsonschema:"how many species to roll, 1-12 (default 3)"`
}

type PreviewOutput struct {
	Species []creaturelab.SpeciesPreview `json:"species"`
}

type ListSpeciesInput struct{}

type ListSpeciesOutput struct {
	Species []creaturelab.SpeciesSummary `json:"species"`
}

type SpeciesIDInput struct {
	ID string `json:"id" jsonschema:"species id"`
}

type SpeciesOutput struct {
	Species creaturelab.SpeciesDetail `json:"species"`
}

type BriefInput struct {
	SpeciesID string `json:"speciesId" jsonschema:"species id"`
	Form      int    `json:"form" jsonschema:"form index from get_species, 0 for a single-form species"`
}

type BriefOutput struct {
	Form  creaturelab.Form `json:"form"`
	Brief string           `json:"brief"`
}

type ListCandidatesInput struct {
	SpeciesID  string `json:"speciesId" jsonschema:"species id"`
	Form       *int   `json:"form,omitempty" jsonschema:"only this form; omit for every form"`
	IncludeSVG bool   `json:"includeSvg,omitempty" jsonschema:"include each candidate's SVG source"`
}

type CandidatesOutput struct {
	Candidates []creaturelab.Candidate `json:"candidates"`
}

type CandidateIDInput struct {
	ID string `json:"id" jsonschema:"sprite candidate id"`
}

type CandidateOutput struct {
	Candidate creaturelab.Candidate `json:"candidate"`
}

type CreateSpeciesInput struct {
	Seed  int64  `json:"seed,omitempty" jsonschema:"seed to roll, usually one from preview_species; omit for a random one"`
	Notes string `json:"notes,omitempty" jsonschema:"optional free-text notes about why it was kept"`
}

type NotesInput struct {
	ID    string `json:"id" jsonschema:"species id"`
	Notes string `json:"notes" jsonschema:"replacement notes"`
}

type SubmitInput struct {
	SpeciesID string `json:"speciesId" jsonschema:"species id"`
	Form      int    `json:"form" jsonschema:"form index the sprite is for"`
	SVG       string `json:"svg" jsonschema:"the complete SVG document"`
	Note      string `json:"note,omitempty" jsonschema:"one or two sentences on what was drawn or changed"`
	Author    string `json:"author,omitempty" jsonschema:"who drew it, e.g. the model name or a person"`
	Accept    bool   `json:"accept,omitempty" jsonschema:"also accept it as the form's sprite (only if the user approved it)"`
}

type OKOutput struct {
	OK bool `json:"ok"`
}

// ---- registration -----------------------------------------------------------

func registerRead(s *mcpsdk.Server, svc *creaturelab.Service) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "preview_species",
		Description: "Roll alien species with the game's own generator without saving them. Free to call; use it to browse before create_species.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in PreviewInput) (*mcpsdk.CallToolResult, PreviewOutput, error) {
		if in.Count == 0 {
			in.Count = 3
		}
		return nil, PreviewOutput{Species: svc.PreviewSpecies(in.Seed, in.Count)}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_species",
		Description: "List saved species, newest first, with how many of their forms have an accepted sprite.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ ListSpeciesInput) (*mcpsdk.CallToolResult, ListSpeciesOutput, error) {
		list, err := svc.ListSpecies(ctx)
		return nil, ListSpeciesOutput{Species: list}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_species",
		Description: "One saved species: its traits, field notes, and each form's sprite slot (accepted sprite and candidate count).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in SpeciesIDInput) (*mcpsdk.CallToolResult, SpeciesOutput, error) {
		sp, err := svc.GetSpecies(ctx, in.ID)
		return nil, SpeciesOutput{Species: sp}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_sprite_brief",
		Description: "The drawing brief for one form of a species: house style, field notes, the form's exact body, and the species' other accepted sprites to stay consistent with. Read it before drawing.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in BriefInput) (*mcpsdk.CallToolResult, BriefOutput, error) {
		sp, err := svc.GetSpecies(ctx, in.SpeciesID)
		if err != nil {
			return nil, BriefOutput{}, err
		}
		brief, err := svc.SpriteBrief(ctx, in.SpeciesID, in.Form)
		if err != nil {
			return nil, BriefOutput{}, err
		}
		return nil, BriefOutput{Form: sp.Forms[in.Form].Form, Brief: brief}, nil
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_sprite_candidates",
		Description: "List the SVGs submitted for a species, newest first within each form, marking the accepted one.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListCandidatesInput) (*mcpsdk.CallToolResult, CandidatesOutput, error) {
		form := -1
		if in.Form != nil {
			form = *in.Form
		}
		list, err := svc.ListCandidates(ctx, in.SpeciesID, form, in.IncludeSVG)
		return nil, CandidatesOutput{Candidates: list}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_sprite_candidate",
		Description: "One sprite candidate with its SVG source and house-style warnings; use it to revise an earlier drawing.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CandidateIDInput) (*mcpsdk.CallToolResult, CandidateOutput, error) {
		c, err := svc.GetCandidate(ctx, in.ID)
		return nil, CandidateOutput{Candidate: c}, err
	})
}

func registerWrite(s *mcpsdk.Server, svc *creaturelab.Service) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "create_species",
		Description: "Roll and save a species (the seed from preview_species gives the same species). Returns its forms, each needing a sprite.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CreateSpeciesInput) (*mcpsdk.CallToolResult, SpeciesOutput, error) {
		sp, err := svc.CreateSpecies(ctx, in.Seed, in.Notes)
		return nil, SpeciesOutput{Species: sp}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "set_species_notes",
		Description: "Replace a species' free-text notes.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in NotesInput) (*mcpsdk.CallToolResult, SpeciesOutput, error) {
		sp, err := svc.SetSpeciesNotes(ctx, in.ID, in.Notes)
		return nil, SpeciesOutput{Species: sp}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "delete_species",
		Description: "Remove a species and its sprites from the catalog. Confirm with the user first.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in SpeciesIDInput) (*mcpsdk.CallToolResult, OKOutput, error) {
		err := svc.DeleteSpecies(ctx, in.ID)
		return nil, OKOutput{OK: err == nil}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "submit_sprite_candidate",
		Description: "Store an SVG as a candidate sprite for one form. Unsafe SVG (scripts, event handlers, external references) is refused; house-style problems come back as warnings. Each call adds a new candidate.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in SubmitInput) (*mcpsdk.CallToolResult, CandidateOutput, error) {
		c, err := svc.SubmitCandidate(ctx, in.SpeciesID, in.Form, in.SVG, in.Note, in.Author, in.Accept)
		return nil, CandidateOutput{Candidate: c}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "accept_sprite_candidate",
		Description: "Make a candidate its form's sprite, replacing any accepted before (which stays as a candidate). Only on the user's approval.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CandidateIDInput) (*mcpsdk.CallToolResult, CandidateOutput, error) {
		c, err := svc.AcceptCandidate(ctx, in.ID)
		return nil, CandidateOutput{Candidate: c}, err
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "delete_sprite_candidate",
		Description: "Remove a sprite candidate. Removing the accepted one leaves its form without a sprite.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CandidateIDInput) (*mcpsdk.CallToolResult, OKOutput, error) {
		err := svc.DeleteCandidate(ctx, in.ID)
		return nil, OKOutput{OK: err == nil}, err
	})
}
