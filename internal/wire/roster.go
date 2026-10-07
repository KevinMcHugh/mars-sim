package wire

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The roster topic: one row per creature the TUI's roster would list
// (internal/ui/tui/render_roster.go, rosterEntries). "roster" is the living
// colonists; "roster:<filters>" adds the TUI's two filters, comma-separated:
// "dead" (every dead colonist, and the recently dead of the other kinds the
// other filter admits) and "nonhuman" (aliens, cats and rats). Each filter
// set is its own topic, so switching one is an unsubscribe and a subscribe.

// rosterEvery is how often the roster is rebuilt. Its rows carry state and
// mood, which change every few ticks, so it would be sent every time; half a
// second keeps a list of hundreds cheap to diff in the page.
const rosterEvery = 500 * time.Millisecond

// RosterRow is one creature in the roster.
type RosterRow struct {
	ID    uint64 `json:"id"`
	Glyph string `json:"glyph"`
	// Look is Glyph in the colonist's own skin and hair, when it has one
	// (see HelloGlyphs.Looks).
	Look glyphs.Look `json:"look,omitempty"`
	Name string      `json:"name"`
	// Info is the second line: "she/her · age 34" for a colonist, an alien's
	// species label, or the kind.
	Info string `json:"info"`
	// State is the third: what it is doing and, for a colonist, its mood; or
	// "dead — <cause>".
	State string `json:"state"`
	Kind  string `json:"kind"`
	Dead  bool   `json:"dead"`
	HP    int    `json:"hp"`
	MaxHP int    `json:"maxHp"`
}

// rosterParam parses the filters after "roster:".
func rosterParam(arg string) (topic, bool) {
	var dead, nonhuman bool
	if arg != "" {
		for _, f := range strings.Split(arg, ",") {
			switch f {
			case "dead":
				dead = true
			case "nonhuman":
				nonhuman = true
			default:
				return topic{}, false
			}
		}
	}
	return topic{every: rosterEvery, build: func(s *sim.Snapshot) any { return rosterTopic(s, dead, nonhuman) }}, true
}

// rosterTopic lists what the TUI's roster lists for these filters, by ID.
func rosterTopic(s *sim.Snapshot, dead, nonhuman bool) []RosterRow {
	include := func(k sim.Kind) bool { return k == sim.Colonist || nonhuman }
	var es []sim.EntityView
	for _, e := range s.Entities {
		if include(e.Kind) {
			es = append(es, e)
		}
	}
	if dead {
		// Colonists from Deceased only: Graveyard has them too, and one death
		// must not be listed twice.
		for _, e := range s.Graveyard {
			if e.Kind != sim.Colonist && include(e.Kind) {
				es = append(es, e)
			}
		}
		for _, e := range s.Deceased {
			es = append(es, e)
		}
	}
	// Deceased is a map: sorting is what makes the order its iteration order
	// can't decide.
	slices.SortFunc(es, func(a, b sim.EntityView) int { return cmp.Compare(a.ID, b.ID) })

	rows := make([]RosterRow, 0, len(es))
	for _, e := range es {
		rows = append(rows, rosterRow(e))
	}
	return rows
}

func rosterRow(e sim.EntityView) RosterRow {
	r := RosterRow{
		ID:    uint64(e.ID),
		Name:  entityName(e),
		Info:  e.Kind.String(),
		Kind:  e.Kind.String(),
		Dead:  e.Dead,
		HP:    e.HP,
		MaxHP: e.MaxHP,
	}
	// A colonist's roster glyph ignores its passing state (the state line
	// says that); other creatures have no resting glyph, so they use the
	// map's.
	if e.Kind == sim.Colonist {
		r.Glyph = glyphs.ForColonist(e.Profile)
		r.Look = glyphs.ForColonistLook(e.Profile)
	} else {
		r.Glyph = glyphs.ForEntity(e)
	}
	if p := e.Profile; p != nil {
		age := "age ?"
		if p.Age > 0 {
			age = "age " + strconv.Itoa(p.Age)
		}
		r.Info = p.Gender.Pronouns() + " · " + age
	} else if e.Kind == sim.Alien {
		r.Info = alienLabel(e)
	}
	r.State = e.State.String()
	if e.State == sim.Idle {
		r.State = "idling"
	}
	if e.Dead {
		r.State = "dead — " + e.Cause
	} else if e.MoodLabel != "" {
		r.State += " · " + e.MoodLabel
	}
	return r
}

// alienLabel is an alien's species label, with its stage of life or caste
// when it has one: "Grelk · hostile · queen".
func alienLabel(e sim.EntityView) string {
	if e.AlienForm == "" {
		return e.AlienSpecies.RosterLabel()
	}
	return e.AlienSpecies.RosterLabel() + " · " + e.AlienForm
}
