package wire

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The news topic lists each broadcast with its day and clock, and its items
// with a kind and, where they happened somewhere, a place.
func TestNewsTopic(t *testing.T) {
	snap := fixture(true)
	snap.NewsHour = 21
	snap.TicksPerDay = 1080
	tick := snap.TicksPerDay*2 + snap.TicksPerDay*15/24 // day 3 21:00, landing at 06:00
	snap.News = []sim.NewsBroadcast{{Tick: tick, Items: []sim.NewsLine{
		{Text: "Cy Ortega is dead: starved.", Kind: "death", Place: sim.Point{X: 4, Y: 5, Level: 1}, HasPlace: true},
		{Text: "1 recruit arrived from off-world: Bo.", Kind: "arrival"},
	}}}
	tp := NewTopics()
	if err := tp.Subscribe("news"); err != nil {
		t.Fatal(err)
	}
	var news NewsTopic
	if err := json.Unmarshal(tp.Due(snap, time.Unix(0, 0))["news"], &news); err != nil {
		t.Fatal(err)
	}
	want := NewsTopic{Hour: 21, Broadcasts: []NewsBroadcast{{
		Tick: tick, Day: 3, When: sim.Clock(tick, snap.TicksPerDay),
		Items: []NewsItem{
			{Text: "Cy Ortega is dead: starved.", Kind: "death", Place: &LogPlace{X: 4, Y: 5, Level: 1}},
			{Text: "1 recruit arrived from off-world: Bo.", Kind: "arrival"},
		},
	}}}
	if !reflect.DeepEqual(news, want) {
		t.Fatalf("news = %+v\nwant %+v", news, want)
	}
	if news.Broadcasts[0].When != "Day 3 21:00" {
		t.Fatalf("aired %q", news.Broadcasts[0].When)
	}
}

// The market topic lists each bodyguard's contract: client, who is on
// station, the rate, and the hours served and left.
func TestMarketTopicListsEscorts(t *testing.T) {
	snap := fixture(true)
	snap.Entities = append(snap.Entities,
		sim.EntityView{ID: 901, Kind: sim.Colonist, Profile: &sim.Profile{Name: "Ada Rich"}},
		sim.EntityView{ID: 902, Kind: sim.Colonist, Profile: &sim.Profile{Name: "Bo Guard"}})
	snap.Economy.WorkOrders = []sim.WorkOrderView{
		{ID: 1, Kind: sim.WorkEscort, Issuer: sim.ColonistOwner(901), Pay: 7, Units: 5, Escort: 902, Served: 3},
		{ID: 2, Kind: sim.WorkEscort, Issuer: sim.ColonistOwner(902), Pay: 6, Units: 8},
	}
	m := marketTopic(snap)
	want := []EscortLine{
		{Client: Person{ID: 901, Name: "Ada Rich"}, Escort: &Person{ID: 902, Name: "Bo Guard"}, Rate: 7, Served: 3, Left: 5, Held: 35},
		{Client: Person{ID: 902, Name: "Bo Guard"}, Rate: 6, Left: 8, Held: 48},
	}
	if !reflect.DeepEqual(m.Escorts, want) {
		t.Fatalf("escorts %+v", m.Escorts)
	}
}
