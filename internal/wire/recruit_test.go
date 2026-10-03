package wire

import (
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// advanceUntil runs eng until a snapshot satisfies ok.
func advanceUntil(t *testing.T, eng *sim.Engine, ok func(*sim.Snapshot) bool) *sim.Snapshot {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		if s, _ := eng.Advance(20 * time.Millisecond); s != nil && ok(s) {
			return s
		}
	}
	t.Fatal("the engine never got there")
	return nil
}

// The Recruit tab carries the recruiter's terms and a card per candidate,
// and hiring from it brings the candidates in under the names on their cards.
func TestRecruitTopic(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 80, 60
	eng := sim.NewEngine(cfg)

	var empty RecruitTopic
	due(t, advanceUntil(t, eng, func(*sim.Snapshot) bool { return true }), "recruit", &empty)
	if empty.Offer != 0 || len(empty.Candidates) != 0 || empty.Fee != cfg.RecruiterFee || empty.SetSize != cfg.RecruitCandidates || !empty.Ready {
		t.Fatalf("before rolling: %+v", empty)
	}

	eng.Send(sim.RollRecruits{})
	var r RecruitTopic
	rolled := advanceUntil(t, eng, func(s *sim.Snapshot) bool { return s.Recruiting.Offer != 0 })
	due(t, rolled, "recruit", &r)
	if r.Offer != 1 || len(r.Candidates) != cfg.RecruitCandidates || r.Treasury != int64(rolled.Economy.Treasury) {
		t.Fatalf("after rolling: offer %d, %d candidates, treasury %d", r.Offer, len(r.Candidates), r.Treasury)
	}
	for i, c := range r.Candidates {
		if c.Index != i || c.Name == "" || c.Glyph == "" || c.Pronouns == "" || c.Age < 18 || c.Height == "" || c.Savings < 0 {
			t.Fatalf("card %d = %+v", i, c)
		}
	}

	eng.Send(sim.HireRecruits{Offer: r.Offer, Picks: []int{1, 2}})
	snap := advanceUntil(t, eng, func(s *sim.Snapshot) bool { return s.Recruiting.Hired == 2 })
	var roster []RosterRow
	due(t, snap, "roster", &roster)
	found := 0
	for _, row := range roster {
		if row.Name == r.Candidates[1].Name || row.Name == r.Candidates[2].Name {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("found %d of the 2 hires on the roster", found)
	}
	var m MarketTopic
	due(t, snap, "market", &m)
	if m.Supply.Exported != cfg.RecruiterFee+2*cfg.RecruitCost {
		t.Fatalf("exported %d, want the fee and two passages", m.Supply.Exported)
	}
}
