// Package export holds the endpoint that hands the catalog to a game build.
package export

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	apigen "github.com/kevinmchugh/mars-sim/internal/creaturelab/api/gen"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
)

// Export is the catalog in the form a game build would load: each species as
// rolled (the sim.AlienSpecies JSON, unchanged) with its accepted sprites.
type Export struct {
	ExportedAt time.Time `json:"exportedAt"`
	Species    []Species `json:"species"`
}

// Species is one species in an Export.
type Species struct {
	ID           string          `json:"id"`
	Seed         int64           `json:"seed"`
	GeneratorRev string          `json:"generatorRev"`
	Complete     bool            `json:"complete"`
	Species      json.RawMessage `json:"species"`
	Sprites      []Sprite        `json:"sprites"`
}

// Sprite is one accepted sprite.
type Sprite struct {
	Form int    `json:"form"`
	Name string `json:"name"`
	SVG  string `json:"svg"`
}

type Store interface {
	ListSpecies(ctx context.Context) ([]db.ListSpeciesRow, error)
	ListAllAccepted(ctx context.Context) ([]db.SpriteCandidate, error)
}

// Endpoint lists species that have a sprite for every form, or every
// species when all is set.
type Endpoint struct{ Store Store }

func (e Endpoint) Interact(ctx context.Context, req apigen.ExportCatalogRequestObject) (Export, error) {
	rows, err := e.Store.ListSpecies(ctx)
	if err != nil {
		return Export{}, err
	}
	acc, err := e.Store.ListAllAccepted(ctx)
	if err != nil {
		return Export{}, err
	}
	sprites := map[string][]db.SpriteCandidate{}
	for _, c := range acc {
		sprites[c.SpeciesID] = append(sprites[c.SpeciesID], c)
	}
	out := Export{ExportedAt: time.Now().UTC(), Species: []Species{}}
	for _, r := range rows {
		complete := int(r.AcceptedCount) >= int(r.FormCount)
		if !complete && !req.Params.All {
			continue
		}
		sp, err := creaturelab.DecodeSpecies(db.Species{ID: r.ID, Data: r.Data})
		if err != nil {
			return Export{}, err
		}
		forms := creaturelab.Forms(sp)
		es := Species{ID: r.ID, Seed: r.Seed, GeneratorRev: r.GeneratorRev, Complete: complete, Species: r.Data, Sprites: []Sprite{}}
		for _, c := range sprites[r.ID] {
			name := ""
			if int(c.Form) < len(forms) {
				name = forms[c.Form].Name
			}
			es.Sprites = append(es.Sprites, Sprite{Form: int(c.Form), Name: name, SVG: c.Svg})
		}
		out.Species = append(out.Species, es)
	}
	return out, nil
}

func (Endpoint) Build(x Export) Export { return x }

func (Endpoint) Render(x Export) apigen.ExportCatalogResponseObject {
	species := make([]apigen.ExportSpecies, len(x.Species))
	for i, s := range x.Species {
		sprites := make([]apigen.ExportSprite, len(s.Sprites))
		for j, p := range s.Sprites {
			sprites[j] = apigen.ExportSprite{Form: p.Form, Name: p.Name, Svg: p.SVG}
		}
		species[i] = apigen.ExportSpecies{
			Id:           s.ID,
			Seed:         s.Seed,
			GeneratorRev: s.GeneratorRev,
			Complete:     s.Complete,
			Species:      s.Species,
			Sprites:      sprites,
		}
	}
	return apigen.ExportCatalog200JSONResponse{ExportedAt: x.ExportedAt, Species: species}
}
