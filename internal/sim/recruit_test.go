package sim

import (
	"slices"
	"testing"
)

// recruitWorld is a test world with recruiting at its defaults and traits on,
// so a candidate's card has something to show.
func recruitWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig()
	cfg.TraitChance = DefaultConfig().TraitChance
	return newTestWorld(t, cfg)
}

// nearShip reports whether p is within d tiles of ship s's footprint box.
func nearShip(s *Ship, p Point, d int) bool {
	return p.X >= s.Origin.X-d && p.Y >= s.Origin.Y-d &&
		p.X < s.Origin.X+s.layout.width+d && p.Y < s.Origin.Y+s.layout.height+d
}

// Rolling pays the recruiter off-world and presents a set; hiring pays each
// passage off-world, and the recruits arrive as the people their cards
// showed, carrying their savings and meals, beside a ship. The money supply
// still balances at every step.
func TestRecruitsArriveAsTheirCardsShowed(t *testing.T) {
	w := recruitWorld(t)
	cfg := w.cfg
	before, colonists := w.treasury, w.countKind(Colonist)

	if !w.rollRecruits() {
		t.Fatalf("roll refused; log: %v", w.log.tail(1))
	}
	if w.treasury != before-Money(cfg.RecruiterFee) || w.moneyExported != Money(cfg.RecruiterFee) {
		t.Fatalf("treasury %v (was %v), exported %v after the fee", w.treasury, before, w.moneyExported)
	}
	assertMoneyConserved(t, w)
	v := w.recruitingView()
	if v.Offer != 1 || len(v.Candidates) != cfg.RecruitCandidates {
		t.Fatalf("offer %d with %d candidates, want 1 with %d", v.Offer, len(v.Candidates), cfg.RecruitCandidates)
	}
	names := map[string]bool{}
	for _, c := range v.Candidates {
		if names[c.Profile.Name] || w.nameTaken(c.Profile.Name, 0) {
			t.Fatalf("candidate name %q is not unique", c.Profile.Name)
		}
		names[c.Profile.Name] = true
		if c.Savings < Money(cfg.RecruitSavingsMin) || c.Savings > Money(cfg.RecruitSavingsMax) {
			t.Fatalf("%s brings %v, outside [%d, %d]", c.Profile.Name, c.Savings, cfg.RecruitSavingsMin, cfg.RecruitSavingsMax)
		}
	}

	picks := []int{3, 0, 3} // out of order, with a repeat: two hires
	cards := []CandidateView{v.Candidates[0], v.Candidates[3]}
	beforeHire, issued := w.treasury, w.moneyIssued
	if !w.hireRecruits(HireRecruits{Offer: v.Offer, Picks: picks}) {
		t.Fatalf("hire refused; log: %v", w.log.tail(1))
	}
	if w.recruits != nil {
		t.Fatal("the set is still on offer after hiring")
	}
	if w.treasury != beforeHire-2*Money(cfg.RecruitCost) {
		t.Fatalf("treasury %v, want %v less 2 × %d", w.treasury, beforeHire, cfg.RecruitCost)
	}
	if got := w.countKind(Colonist); got != colonists+2 {
		t.Fatalf("%d colonists, want %d", got, colonists+2)
	}
	minted := Money(0)
	for i, card := range cards {
		e := w.entities[EntityID(int(w.nextID)-2+i)]
		if e == nil || e.Kind != Colonist || e.Profile.Name != card.Profile.Name || e.Profile.Age != card.Profile.Age ||
			!slices.Equal(e.Profile.Traits, card.Profile.Traits) || e.Profile.HeightCM != card.Profile.HeightCM {
			t.Fatalf("recruit %d is %+v, card showed %+v", i, e.Profile, card.Profile)
		}
		if got := e.skillViews(); !slices.Equal(got, card.Skills) {
			t.Fatalf("%s has skills %v, card showed %v", e.displayName(), got, card.Skills)
		}
		if e.wallet != card.Savings {
			t.Fatalf("%s holds %v, card showed savings of %v", e.displayName(), e.wallet, card.Savings)
		}
		minted += card.Savings
		if got := e.ownCarried(Meal); got != cfg.RecruitMeals {
			t.Fatalf("%s carries %d meals, want %d", e.displayName(), got, cfg.RecruitMeals)
		}
		if len(w.relativesOf(e, w.cachedKinChildren())) != 0 {
			t.Fatalf("%s arrived with family in the colony", e.displayName())
		}
		near := false
		for _, s := range w.ships {
			near = near || nearShip(s, e.Pos, 6)
		}
		if !near {
			t.Fatalf("%s arrived at %v, nowhere near a ship", e.displayName(), e.Pos)
		}
	}
	if w.moneyIssued != issued+minted {
		t.Fatalf("issued %v, want %v plus the savings %v", w.moneyIssued, issued, minted)
	}
	assertMoneyConserved(t, w)
	for range 200 {
		w.step()
	}
	assertMoneyConserved(t, w)
}

// A roll the treasury cannot pay for, a hire it cannot pay for, and a hire
// naming a set no longer on offer all leave everything as it was. Hiring
// nobody turns the set away.
func TestRecruitingRefusesWhatItCannotDo(t *testing.T) {
	w := recruitWorld(t)
	// setTreasury keeps the books balanced while the test sets the balance.
	setTreasury := func(m Money) { w.moneyIssued += m - w.treasury; w.treasury = m }
	setTreasury(Money(w.cfg.RecruiterFee) - 1)
	if w.rollRecruits() || w.recruits != nil || w.moneyExported != 0 {
		t.Fatal("a roll beyond the treasury went through")
	}
	setTreasury(Money(w.cfg.RecruiterFee) + Money(w.cfg.RecruitCost))
	if !w.rollRecruits() {
		t.Fatal("an affordable roll was refused")
	}
	colonists := w.countKind(Colonist)
	if w.hireRecruits(HireRecruits{Offer: 1, Picks: []int{0, 1}}) || w.recruits == nil || w.countKind(Colonist) != colonists {
		t.Fatal("a hire beyond the treasury went through, or closed the set")
	}
	if w.hireRecruits(HireRecruits{Offer: 2, Picks: []int{0}}) || w.countKind(Colonist) != colonists {
		t.Fatal("a hire from a set never rolled went through")
	}
	if w.hireRecruits(HireRecruits{Offer: 1, Picks: []int{99}}) || w.recruits != nil {
		t.Fatal("hiring nobody did not turn the set away")
	}
	if w.treasury != Money(w.cfg.RecruitCost) {
		t.Fatalf("treasury %v, want only the fee spent", w.treasury)
	}
	assertMoneyConserved(t, w)
}

// Re-rolling pays the fee again and replaces the set; the old set's id no
// longer hires.
func TestARerollReplacesTheSet(t *testing.T) {
	w := recruitWorld(t)
	w.rollRecruits()
	first := w.recruitingView().Candidates[0].Profile.Name
	w.rollRecruits()
	v := w.recruitingView()
	if v.Offer != 2 || w.moneyExported != 2*Money(w.cfg.RecruiterFee) {
		t.Fatalf("offer %d, exported %v after two rolls", v.Offer, w.moneyExported)
	}
	if v.Candidates[0].Profile.Name == first {
		t.Fatalf("the re-roll presented %s again", first)
	}
	if w.hireRecruits(HireRecruits{Offer: 1, Picks: []int{0}}) {
		t.Fatal("hired from the replaced set")
	}
}

// Recruiting draws only from its own stream (and the simulation stream's
// usual draws for a new arrival's needs): rolling sets never changes who the
// next ship brings.
func TestRollingRecruitsShiftsNoOtherStream(t *testing.T) {
	a, b := recruitWorld(t), recruitWorld(t)
	for range 3 {
		b.rollRecruits()
	}
	sa, sb := a.land(1, false), b.land(1, false)
	pa, pb := a.entities[sa.Colonists[0]], b.entities[sb.Colonists[0]]
	if pa.Profile.Name != pb.Profile.Name || pa.practice != pb.practice || pa.Pos != pb.Pos {
		t.Fatalf("the next arrival differs once recruits were rolled: %s at %v vs %s at %v",
			pa.Profile.Name, pa.Pos, pb.Profile.Name, pb.Pos)
	}
}

// The same seed rolls the same candidates.
func TestRecruitsAreDeterministic(t *testing.T) {
	a, b := recruitWorld(t), recruitWorld(t)
	a.rollRecruits()
	b.rollRecruits()
	va, vb := a.recruitingView(), b.recruitingView()
	for i := range va.Candidates {
		ca, cb := va.Candidates[i], vb.Candidates[i]
		if ca.Profile.Name != cb.Profile.Name || ca.Savings != cb.Savings || !slices.Equal(ca.Skills, cb.Skills) {
			t.Fatalf("candidate %d: %s ($%d) vs %s ($%d)", i, ca.Profile.Name, ca.Savings, cb.Profile.Name, cb.Savings)
		}
	}
}

// The issue's two tunings: a $100 mean with a $50 spread keeps about 95% of
// candidates within $50 of $100, and a $1,000 mean with a huge spread leaves
// most candidates below the mean and a rare few far above it. Neither ever
// rolls debt.
func TestRecruitSavingsFollowTheirTuning(t *testing.T) {
	const n = 20000
	r := newRand(7)
	cfg := DefaultConfig()
	cfg.RecruitSavingsMean, cfg.RecruitSavingsSpread = 100, 50
	cfg.RecruitSavingsMin, cfg.RecruitSavingsMax = 0, 1_000_000
	within, sum := 0, Money(0)
	for range n {
		m := rollSavings(r, cfg)
		sum += m
		if m >= 50 && m <= 150 {
			within++
		}
	}
	if pct := float64(within) / n * 100; pct < 93.5 || pct > 96.5 {
		t.Fatalf("%.1f%% within $50 of $100, want about 95%%", pct)
	}
	if mean := float64(sum) / n; mean < 98 || mean > 102 {
		t.Fatalf("mean $%.1f, want about $100", mean)
	}

	cfg.RecruitSavingsMean, cfg.RecruitSavingsSpread = 1000, 10000
	below, rich, sum := 0, 0, Money(0)
	for range n {
		m := rollSavings(r, cfg)
		if m < 0 {
			t.Fatalf("rolled debt: %v", m)
		}
		sum += m
		if m < 1000 {
			below++
		}
		if m >= 10000 {
			rich++
		}
	}
	if below < n*3/4 {
		t.Fatalf("%d of %d below the $1000 mean, want most of them", below, n)
	}
	if rich == 0 || rich > n/20 {
		t.Fatalf("%d of %d brought $10,000 or more, want a rare few", rich, n)
	}
	if mean := float64(sum) / n; mean < 700 || mean > 1300 {
		t.Fatalf("mean $%.0f, want about $1000", mean)
	}

	cfg.RecruitSavingsMin, cfg.RecruitSavingsMax = 900, 1100
	for range 1000 {
		if m := rollSavings(r, cfg); m < 900 || m > 1100 {
			t.Fatalf("rolled %v outside the clamp [900, 1100]", m)
		}
	}
	cfg.RecruitSavingsSpread = 0
	cfg.RecruitSavingsMin, cfg.RecruitSavingsMax = 0, 5000
	if m := rollSavings(r, cfg); m != 1000 {
		t.Fatalf("no spread rolled %v, want the mean", m)
	}
}

// Hiring waits for the founders to land: there is nowhere to put anyone
// while their ships are aloft.
func TestRecruitsWaitForTheFounders(t *testing.T) {
	cfg := testConfig()
	cfg.PlaceShips = true
	w := newTestWorld(t, cfg)
	if len(w.aloft) == 0 {
		t.Skip("no ships aloft with place-ships")
	}
	w.rollRecruits()
	if w.hireRecruits(HireRecruits{Offer: 1, Picks: []int{0}}) || w.recruits == nil {
		t.Fatal("hired while the founders were aloft")
	}
	if w.recruitingView().Ready {
		t.Fatal("the desk says ready while the founders are aloft")
	}
}
