package sim

import (
	"fmt"
	"slices"
	"strings"
)

// The nightly news: once a day, at news-hour, the colony hears the day's
// major news. Any system with news calls reportNews where the thing happens;
// today that is a colonist dying (remove) and someone arriving (a ship
// landing, recruits hired). Each colonist hears what has aired since it last
// listened as a percept on the broadcast channel, the first time it is awake
// after the broadcast, so what hearing it does is cognition.yaml rows. See
// docs/safety.md.

// NewsItem is one piece of news: the occurrence it reports, in the
// perception grammar, and the line the colony hears.
type NewsItem struct {
	Seq    int // its place in the colony's news, from 0
	Tick   int
	Actor  FactRef
	Action ActionID
	Object FactRef
	Text   string
	// Place is where it happened, for the News tab to point the map at;
	// HasPlace is false for news about no one place.
	Place    Point
	HasPlace bool
	// Aired is the tick of the broadcast it went out in; 0 until then.
	Aired int
	// newcomers are who the news is about when it is an arrival: they were
	// there, so they do not hear it.
	newcomers []EntityID
}

// NewsBroadcast is one evening's news as it aired, for the News tab.
type NewsBroadcast struct {
	Tick  int
	Items []NewsLine
}

// NewsLine is one item of a broadcast: what was said, what kind of news it
// is ("death", "arrival"), and where it happened, if anywhere.
type NewsLine struct {
	Text     string
	Kind     string
	Place    Point
	HasPlace bool
}

// newsKind names an item's kind for the News tab, from its action.
func newsKind(a ActionID) string {
	switch a {
	case ActionDie:
		return "death"
	case ActionArrive:
		return "arrival"
	default:
		return string(a)
	}
}

// publishedNewsBroadcasts is every broadcast still kept, newest first, as
// snapshots publish it: built when something airs or is dropped, and shared
// by every snapshot until then, so nothing may modify it.
func (w *World) publishedNewsBroadcasts() []NewsBroadcast {
	if w.publishedNewsRev == w.newsRev+1 {
		return w.publishedNews
	}
	var out []NewsBroadcast
	for i := len(w.news) - 1; i >= 0; i-- {
		it := w.news[i]
		if it.Aired == 0 {
			continue
		}
		if len(out) == 0 || out[len(out)-1].Tick != it.Aired {
			out = append(out, NewsBroadcast{Tick: it.Aired})
		}
		b := &out[len(out)-1]
		b.Items = append(b.Items, NewsLine{Text: it.Text, Kind: newsKind(it.Action), Place: it.Place, HasPlace: it.HasPlace})
	}
	for i := range out { // items in the order they were reported
		slices.Reverse(out[i].Items)
	}
	w.publishedNews, w.publishedNewsRev = out, w.newsRev+1
	return out
}

// reportNews adds an item to the next broadcast.
func (w *World) reportNews(item NewsItem) {
	if w.cfg.NewsHour < 0 {
		return
	}
	item.Seq, item.Tick = w.newsSeq, w.tick
	w.newsSeq++
	w.news = append(w.news, item)
}

// newsHourAt is the clock hour tick falls in.
func (w *World) newsHourAt(tick int) int {
	return MinuteOfDay(tick, w.cfg.TicksPerDay()) / 60
}

// airNews broadcasts everything reported since the last broadcast, on the
// first tick of news-hour, and drops items older than news-days that everyone
// has had the chance to hear. step calls it once a tick.
func (w *World) airNews() {
	if w.cfg.NewsHour < 0 || w.newsHourAt(w.tick) != w.cfg.NewsHour || w.newsHourAt(w.tick-1) == w.cfg.NewsHour {
		return
	}
	if w.newsAired < w.newsSeq {
		lines := make([]string, 0, w.newsSeq-w.newsAired)
		for i := range w.news {
			if it := &w.news[i]; it.Seq >= w.newsAired {
				lines = append(lines, it.Text)
				it.Aired = w.tick
			}
		}
		w.newsAired = w.newsSeq
		w.newsRev++
		w.logEvent(LogNote, fmt.Sprintf("Evening news, day %d: %s", DayOf(w.tick, w.cfg.TicksPerDay()), strings.Join(lines, " ")))
	}
	keep := w.tick - max(w.cfg.NewsDays, 1)*w.cfg.TicksPerDay()
	if n := len(w.news); n > 0 {
		w.news = slices.DeleteFunc(w.news, func(it NewsItem) bool { return it.Seq < w.newsAired && it.Tick < keep })
		if len(w.news) != n {
			w.newsRev++
		}
	}
}

// hearNews gives e the news that has aired since it last listened, if it is
// awake to hear it. step calls it every colonist turn; with nothing new it
// costs one comparison.
func (w *World) hearNews(e *Entity) {
	if e.newsHeard >= w.newsAired || e.State == Sleeping || e.passedOut || !e.Alive() {
		return
	}
	for _, it := range w.news {
		if it.Seq < e.newsHeard || it.Seq >= w.newsAired || slices.Contains(it.newcomers, e.ID) {
			continue
		}
		if w.knows(e, it.Actor, it.Action, it.Object) {
			continue
		}
		o := Occurrence{Actor: it.Actor, Action: it.Action, Object: it.Object, Location: e.Pos,
			Text: "Heard on the evening news: " + it.Text}
		w.rememberPercept(e, w.perceptFor(e, ChannelBroadcast, RoleWitness, PhaseInstant, o))
	}
	e.newsHeard = w.newsAired
}

// reportDeath is the news of a colonist's death, with remove's cause.
func (w *World) reportDeath(e *Entity, cause string) {
	w.reportNews(NewsItem{
		Action: ActionDie, Object: w.factRef(e),
		Text:  fmt.Sprintf("%s is dead: %s.", e.displayName(), cause),
		Place: e.Pos, HasPlace: true,
	})
}

// reportShipArrival is the news of a colony ship landing in play.
func (w *World) reportShipArrival(s *Ship) {
	n := len(s.Colonists)
	text := fmt.Sprintf("A colony ship landed with %d settlers aboard.", n)
	if n == 1 {
		text = fmt.Sprintf("A colony ship landed with %s aboard.", w.entities[s.Colonists[0]].displayName())
	}
	w.reportNews(NewsItem{
		Actor: FactRef{Noun: NounShip, Label: "a colony ship"}, Action: ActionArrive, Text: text,
		Place: s.Origin.Add(s.layout.width/2, s.layout.height/2), HasPlace: true,
		newcomers: slices.Clone(s.Colonists),
	})
}

// reportRecruits is the news of recruits arriving from off-world.
func (w *World) reportRecruits(ids []EntityID, names []string) {
	item := NewsItem{
		Actor: FactRef{Noun: NounColonist, Label: "recruits"}, Action: ActionArrive,
		Text:      fmt.Sprintf("%s from off-world: %s.", pluralize(len(ids), "recruit arrived", "recruits arrived"), strings.Join(names, ", ")),
		newcomers: slices.Clone(ids),
	}
	if len(ids) > 0 {
		if e := w.entities[ids[0]]; e != nil {
			item.Place, item.HasPlace = e.Pos, true
		}
	}
	w.reportNews(item)
}
