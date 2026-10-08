package creaturelab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// MaxPreview bounds one preview call.
const MaxPreview = 12

// MaxDescription bounds rewritten field notes, in characters. The generated
// ones run a few hundred; this leaves room for a much richer rewrite.
const MaxDescription = 4000

// Store is the lab's persistence: every sqlc query, plus the operations that
// need a transaction. Endpoints each declare a narrow interface of just the
// methods they call, and *Store satisfies all of them, as *dbgen.Queries
// does in the inventory app.
type Store struct {
	*db.Queries
	Pool *pgxpool.Pool
}

// NewStore builds a Store on pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Queries: db.New(pool), Pool: pool}
}

// AcceptCandidate makes a candidate its slot's sprite, replacing whichever
// was accepted before (that one stays as a candidate). The clear and the
// mark share a transaction so a slot is never left with two, or none.
func (s *Store) AcceptCandidate(ctx context.Context, id string) (db.SpriteCandidate, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.SpriteCandidate{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit
	q := s.WithTx(tx)
	c, err := q.GetCandidate(ctx, id)
	if err != nil {
		return db.SpriteCandidate{}, NotFound(err, "candidate", id)
	}
	if err := q.ClearAccepted(ctx, db.ClearAcceptedParams{SpeciesID: c.SpeciesID, Form: c.Form}); err != nil {
		return db.SpriteCandidate{}, err
	}
	if c, err = q.MarkAccepted(ctx, id); err != nil {
		return db.SpriteCandidate{}, err
	}
	return c, tx.Commit(ctx)
}

// RedeemInvite spends a single-use invite: it mints the newcomer's api key
// (named after the invite's note) and runs approve with that key on the same
// transaction, so an invite is only used up if what it was redeemed for
// (an OAuth code) is stored too. An unknown, used, expired or deleted invite
// is pgx.ErrNoRows. The raw key is never shown to anyone: the newcomer acts
// through the OAuth tokens it approves, and deleting the key cuts them off.
func (s *Store) RedeemInvite(ctx context.Context, raw string, approve func(q *db.Queries, apiKeyID string) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit
	q := s.WithTx(tx)
	inv, err := q.ClaimInviteByHash(ctx, auth.Hash(raw))
	if err != nil {
		return err
	}
	name := inv.Note
	if name == "" {
		name = "invite " + inv.ID
	}
	key, err := auth.GenerateKey()
	if err != nil {
		return err
	}
	row, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{ID: xid.New().String(), Name: name, KeyHash: auth.Hash(key)})
	if err != nil {
		return err
	}
	if err := q.SetInviteKey(ctx, db.SetInviteKeyParams{ID: inv.ID, ApiKeyID: &row.ID}); err != nil {
		return err
	}
	if err := approve(q, row.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Links builds the absolute URLs that view models carry.
type Links struct {
	// Base is the lab's public origin ("https://creature-lab-xyz.sprites.app").
	// Empty gives relative links.
	Base string
}

// NewLinks trims base for joining.
func NewLinks(base string) Links { return Links{Base: strings.TrimRight(base, "/")} }

// Species is a species' page.
func (l Links) Species(id string) string { return l.Base + "/species/" + id }

// Sprite is a candidate's SVG.
func (l Links) Sprite(id string) string { return l.Base + "/sprites/" + id + ".svg" }

// FieldNotes is the description to show for a stored species: its rewrite
// when it has one, else the text the roster code generated when it rolled
// the species (kept in description as rolled, not regenerated, since later
// roster code may word it differently).
func FieldNotes(row db.Species) string {
	if row.DescriptionOverride != "" {
		return row.DescriptionOverride
	}
	return row.Description
}

// DecodeSpecies loads a row's stored sim.AlienSpecies.
func DecodeSpecies(row db.Species) (sim.AlienSpecies, error) {
	var sp sim.AlienSpecies
	if err := json.Unmarshal(row.Data, &sp); err != nil {
		return sp, fmt.Errorf("species %s: stored data: %w", row.ID, err)
	}
	return sp, nil
}

// NotFound turns pgx's no-rows into ErrNotFound, naming what was missing.
func NotFound(err error, what, id string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s %s", ErrNotFound, what, id)
	}
	return err
}
