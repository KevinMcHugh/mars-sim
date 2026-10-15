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
	// newcomers are who the news is about when it is an arrival: they were
	// there, so they do not hear it.
	newcomers []EntityID
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
		for _, it := range w.news {
			if it.Seq >= w.newsAired {
				lines = append(lines, it.Text)
			}
		}
		w.newsAired = w.newsSeq
		w.logEvent(LogNote, fmt.Sprintf("Evening news, day %d: %s", DayOf(w.tick, w.cfg.TicksPerDay()), strings.Join(lines, " ")))
	}
	keep := w.tick - max(w.cfg.NewsDays, 1)*w.cfg.TicksPerDay()
	w.news = slices.DeleteFunc(w.news, func(it NewsItem) bool { return it.Seq < w.newsAired && it.Tick < keep })
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
		Text: fmt.Sprintf("%s is dead: %s.", e.displayName(), cause),
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
		newcomers: slices.Clone(s.Colonists),
	})
}

// reportRecruits is the news of recruits arriving from off-world.
func (w *World) reportRecruits(ids []EntityID, names []string) {
	w.reportNews(NewsItem{
		Actor: FactRef{Noun: NounColonist, Label: "recruits"}, Action: ActionArrive,
		Text: fmt.Sprintf("%s from off-world: %s.", pluralize(len(ids), "recruit arrived", "recruits arrived"), strings.Join(names, ", ")),

		newcomers: slices.Clone(ids),
	})
}
