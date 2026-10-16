package wire

import (
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The news topic: the News tab's evening broadcasts (sim.NewsBroadcast),
// newest first. It changes only when a broadcast airs or old news is
// dropped, so the JSON dedupe sends it about once a colony day. See
// docs/safety.md.

// NewsTopic is every broadcast the colony still keeps, and the clock hour
// they air at (-1: the news is off).
type NewsTopic struct {
	Hour       int             `json:"hour"`
	Broadcasts []NewsBroadcast `json:"broadcasts"`
}

// NewsBroadcast is one evening's news: when it aired and what it said.
type NewsBroadcast struct {
	Tick  int        `json:"tick"`
	Day   int        `json:"day"`
	When  string     `json:"when"` // Tick as a colony date and time (sim.Clock)
	Items []NewsItem `json:"items"`
}

// NewsItem is one story: its text, its kind ("death", "arrival"), and the
// tile it happened on, which clicking it centers the map on.
type NewsItem struct {
	Text  string    `json:"text"`
	Kind  string    `json:"kind"`
	Place *LogPlace `json:"place,omitempty"`
}

func newsTopic(s *sim.Snapshot) NewsTopic {
	t := NewsTopic{Hour: s.NewsHour, Broadcasts: make([]NewsBroadcast, 0, len(s.News))}
	for _, b := range s.News {
		nb := NewsBroadcast{Tick: b.Tick, Day: sim.DayOf(b.Tick, s.TicksPerDay), When: sim.Clock(b.Tick, s.TicksPerDay),
			Items: make([]NewsItem, 0, len(b.Items))}
		for _, it := range b.Items {
			ni := NewsItem{Text: it.Text, Kind: it.Kind}
			if it.HasPlace {
				ni.Place = &LogPlace{X: it.Place.X, Y: it.Place.Y, Level: int(it.Place.Level)}
			}
			nb.Items = append(nb.Items, ni)
		}
		t.Broadcasts = append(t.Broadcasts, nb)
	}
	return t
}
