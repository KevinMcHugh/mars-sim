package sim

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

// ---- Recruiting ---------------------------------------------------------------
//
// The player grows the colony by hiring from off-world. The treasury pays a
// recruiter recruiter-fee for a set of recruit-candidates candidates, each a
// whole person rolled in advance (name, body, traits, skills) with the savings
// it would bring. The player hires any of them, none, or pays again for a new
// set. Each hire costs recruit-cost from the treasury, and the recruit appears
// at once beside one of the colony's ships with its savings in its wallet and
// recruit-meals meals in its pockets.
//
// The fee and the passage are paid off-world (export): they leave the money
// supply. A recruit's savings are minted into its wallet, so a lucky hire can
// bring in more than it cost. See docs/recruiting.md.

// recruitSeed separates the recruit stream from the others (see rng.go).
const recruitSeed = 0x1F83D9ABFB41BD6B

// RollRecruits pays the recruiter for a new set of candidates, replacing any
// set still on offer.
type RollRecruits struct{}

// HireRecruits hires the candidates at Picks (indices into the set on offer)
// and closes the offer. Offer names the set, so a hire sent just before a
// re-roll cannot hire from the new one. Hiring nobody turns the set away.
type HireRecruits struct {
	Offer int
	Picks []int
}

func (RollRecruits) isCommand() {}
func (HireRecruits) isCommand() {}

// recruitCandidate is one person the recruiter presents: who it will be when
// it arrives, and what it brings.
type recruitCandidate struct {
	profile    *Profile
	practice   [numSkills]uint32
	profession SkillKind
	savings    Money
}

// recruitOffer is the set of candidates on offer. id counts sets rolled,
// from 1.
type recruitOffer struct {
	id         int
	candidates []*recruitCandidate
}

// rollRecruits pays the recruiter and presents a new set, reporting whether
// it did. It logs what happened, or why not.
func (w *World) rollRecruits() bool {
	n, fee := w.cfg.RecruitCandidates, Money(w.cfg.RecruiterFee)
	switch {
	case n <= 0:
		w.logEvent(LogNote, "The colony has no recruiter to call.")
		return false
	case fee > w.treasury:
		w.logEvent(LogNote, fmt.Sprintf("The colony cannot pay the recruiter: the fee is %v and the treasury holds %v.", fee, w.treasury))
		return false
	}
	if !w.export(Community, fee) {
		return false
	}
	w.recruitSets++
	offer := &recruitOffer{id: w.recruitSets}
	taken := map[string]bool{}
	for range n {
		c := w.rollCandidate(taken)
		taken[c.profile.Name] = true
		offer.candidates = append(offer.candidates, c)
	}
	w.recruits = offer
	w.logEvent(LogNote, fmt.Sprintf("The colony pays a recruiter %v, who presents %d candidates.", fee, n))
	return true
}

// rollCandidate draws one candidate from the recruit stream: a profile the
// way a colonist's is rolled, a name nobody in the colony or in taken goes by,
// a background, and savings. Nothing here touches another stream, so how
// often the player rolls changes no one else.
func (w *World) rollCandidate(taken map[string]bool) *recruitCandidate {
	r := w.recruitRNG
	p := rollProfile(r, r, w.cfg.TraitChance)
	for i := 0; i < givenNameRedraws && (taken[p.Name] || w.nameTaken(p.Name, 0)); i++ {
		p.given = rollGivenName(r, p.Gender)
		p.Name = p.given + " " + p.surname
	}
	// The background lands on a scratch colonist, which is all setRank and
	// updateProfession need.
	scratch := &Entity{Kind: Colonist, Profile: p}
	w.rollBackgroundFrom(r, scratch)
	return &recruitCandidate{
		profile:    p,
		practice:   scratch.practice,
		profession: scratch.profession,
		savings:    rollSavings(r, w.cfg),
	}
}

// rollSavings draws what a candidate brings: log-normal, with mean
// recruit-savings-mean and a standard deviation of recruit-savings-spread /
// 1.96, so while the spread is well under the mean about 95% of candidates
// land within the spread of it. A spread as wide as the mean or wider skews
// it: most candidates bring little and a rare few a fortune. Clamped to
// [recruit-savings-min, recruit-savings-max], never below 0.
func rollSavings(r *rand.Rand, cfg Config) Money {
	lo, hi := max(cfg.RecruitSavingsMin, 0), cfg.RecruitSavingsMax
	mean, sd := float64(cfg.RecruitSavingsMean), float64(cfg.RecruitSavingsSpread)/1.96
	v := mean
	if mean > 0 && sd > 0 {
		sigma2 := math.Log1p(sd * sd / (mean * mean))
		mu := math.Log(mean) - sigma2/2
		v = math.Exp(mu + math.Sqrt(sigma2)*r.NormFloat64())
	}
	m := int64(math.Round(v))
	if hi >= lo {
		m = min(m, hi)
	}
	return Money(max(m, lo))
}

// hireRecruits hires the picked candidates from the set on offer and reports
// whether anyone arrived. Hiring nobody turns the set away. A hire the
// treasury cannot pay for in full, or with nowhere to put everyone, hires
// nobody and leaves the set on offer.
func (w *World) hireRecruits(c HireRecruits) bool {
	o := w.recruits
	if o == nil || o.id != c.Offer {
		return false // a stale command: the set it named is gone
	}
	var picks []int
	for _, i := range c.Picks {
		if i >= 0 && i < len(o.candidates) && !slices.Contains(picks, i) {
			picks = append(picks, i)
		}
	}
	slices.Sort(picks)
	if len(picks) == 0 {
		w.recruits = nil
		w.logEvent(LogNote, "The colony turns the recruiter's candidates away.")
		return false
	}
	refuse := func(why string) bool {
		w.logEvent(LogNote, fmt.Sprintf("The colony cannot hire %d recruits: %s.", len(picks), why))
		return false
	}
	cost := Money(w.cfg.RecruitCost) * Money(len(picks))
	if cost > w.treasury {
		return refuse(fmt.Sprintf("their passage costs %v and the treasury holds %v", cost, w.treasury))
	}
	if len(w.aloft) > 0 {
		return refuse("the founders' ships have not landed yet")
	}
	at := w.recruitLanding(len(picks))
	if len(at) < len(picks) {
		return refuse("there is no room for them near the ships")
	}
	if !w.export(Community, cost) {
		return false
	}
	names := make([]string, 0, len(picks))
	brought := Money(0)
	for i, pick := range picks {
		e := w.arriveRecruit(o.candidates[pick], at[i])
		names = append(names, e.displayName())
		brought += e.wallet
	}
	w.recruits = nil
	w.recruitsHired += len(picks)
	w.logEvent(LogArrival, fmt.Sprintf("%s from off-world for %v, bringing %v: %s.",
		pluralize(len(picks), "recruit arrives", "recruits arrive"), cost, brought, strings.Join(names, ", ")))
	return true
}

// arriveRecruit brings one hired candidate into the world at p.
func (w *World) arriveRecruit(c *recruitCandidate, p Point) *Entity {
	e := w.spawnWith(Colonist, p, 0, c)
	e.practice = c.practice
	e.profession = c.profession
	w.rollEmployer(e)
	if n := w.cfg.RecruitMeals; n > 0 {
		e.Inventory.Add(Meal, n)
	}
	return e
}

// recruitLanding finds up to n empty floor tiles in a cluster beside one of
// the colony's ships, picked on the recruit stream: a breadth-first walk out
// from the ship's aisle over walkable ground, taking plain floor nobody
// stands on and no doorway approach. A ship that has been cleared away still
// marks the spot. It returns fewer than n only when the walk runs out of
// room.
func (w *World) recruitLanding(n int) []Point {
	if len(w.ships) == 0 {
		return nil
	}
	s := w.ships[w.recruitRNG.IntN(len(w.ships))]
	var seeds []Point
	for _, d := range s.layout.floor {
		if p := s.Origin.Add(d.X, d.Y); w.Walkable(p) {
			seeds = append(seeds, p)
		}
	}
	if len(seeds) == 0 {
		// Nothing of the aisle is walkable: start from the nearest
		// walkable tile to the ship's corner instead.
		for r := 0; r <= recruitSearchRadius && len(seeds) == 0; r++ {
			forEachRingPoint(s.Origin, r, func(p Point) bool {
				if w.Walkable(p) {
					seeds = append(seeds, p)
					return true // found: stop the ring
				}
				return false
			})
		}
	}
	seen := map[Point]bool{}
	queue := slices.Clone(seeds)
	for _, p := range seeds {
		seen[p] = true
	}
	var out []Point
	for len(queue) > 0 && len(out) < n && len(seen) < recruitSearchTiles {
		p := queue[0]
		queue = queue[1:]
		if w.TerrainAt(p) == Floor && !w.occupied(p) && !w.doorTiles[p] {
			out = append(out, p)
		}
		for _, d := range [4]gridStep{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
			q := p.Add(d.X, d.Y)
			if !seen[q] && w.Walkable(q) {
				seen[q] = true
				queue = append(queue, q)
			}
		}
	}
	return out
}

const (
	// recruitSearchRadius is how far from a ship's corner recruitLanding
	// looks for walkable ground when none of its aisle is.
	recruitSearchRadius = 30
	// recruitSearchTiles caps the walk, so a crowded colony costs a bounded
	// search rather than the whole map.
	recruitSearchTiles = 4096
)

// ---- Snapshot -----------------------------------------------------------------

// RecruitingView is the recruiting desk for a frame: the terms, and the set
// on offer, if any.
type RecruitingView struct {
	Fee        Money // the recruiter's fee for a set
	Cost       Money // passage for one hire
	SetSize    int   // candidates in a set; 0: recruiting is off
	Meals      int   // meals each recruit arrives with
	Offer      int   // the set on offer's id, 0 for none
	Candidates []CandidateView
	// Ready is false while the founders' ships are still aloft: hiring has
	// nowhere to put anyone yet.
	Ready bool
	// Sets is how many sets have been rolled, Hired how many recruits have
	// arrived.
	Sets  int
	Hired int
}

// CandidateView is one candidate: who it will be, and what it brings.
type CandidateView struct {
	Index           int
	Profile         *Profile
	Skills          []SkillView
	Profession      SkillKind
	ProfessionLabel string
	Savings         Money
}

func (w *World) recruitingView() RecruitingView {
	v := RecruitingView{
		Fee: Money(w.cfg.RecruiterFee), Cost: Money(w.cfg.RecruitCost),
		SetSize: max(w.cfg.RecruitCandidates, 0), Meals: max(w.cfg.RecruitMeals, 0),
		Ready: len(w.aloft) == 0, Sets: w.recruitSets, Hired: w.recruitsHired,
	}
	if o := w.recruits; o != nil {
		v.Offer = o.id
		for i, c := range o.candidates {
			scratch := &Entity{Kind: Colonist, Profile: c.profile, practice: c.practice, profession: c.profession}
			v.Candidates = append(v.Candidates, CandidateView{
				Index: i, Profile: c.profile.clone(), Skills: scratch.skillViews(),
				Profession: c.profession, ProfessionLabel: scratch.professionLabel(), Savings: c.savings,
			})
		}
	}
	return v
}
