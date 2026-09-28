package sim

import (
	"fmt"
	"math"
	"slices"
)

// ---- Skills ------------------------------------------------------------------
//
// A colonist gets better at work by doing it. Practice is counted in base work
// ticks of completed work, per skill; a skill's rank is where that practice
// sits on the skill's curve, and each rank has a label and an effect on how
// fast the work goes and how much it yields. Ranks are few and named: other
// systems ask for a rank or a label, never for the practice count. See
// docs/skills.md.

// SkillKind is a kind of work a colonist can get better at. Recipes name the
// one they call for; mining, foraging and construction are credited where
// that work completes.
type SkillKind uint8

const (
	SkillNone SkillKind = iota
	SkillMining
	SkillForaging
	SkillCooking
	SkillConstruction
	SkillSmithing
	numSkills
)

func (k SkillKind) String() string {
	if k < numSkills {
		return skillSpecs[k].Name
	}
	return fmt.Sprintf("skill(%d)", k)
}

// rankEffect is what a rank does to the work. TicksPct is the percent of the
// base work ticks the work takes (100: unchanged); YieldPct is the percent of
// a recipe's normal output it makes (100: unchanged). Throughput is YieldPct /
// TicksPct.
type rankEffect struct {
	TicksPct int
	YieldPct int
}

// skillSpec is one skill's curve. Rank r >= 1 is reached at Unit × Base^(r-1)
// ticks of practice, up to the last label: rank = ⌊log_Base(practice/Unit)⌋+1,
// computed by walking the thresholds in integers (a float log floors exact
// powers wrong: math.Log(1000)/math.Log(10) is 2.9999999999999996). Labels
// and Effects have one entry per rank, rank 0 ("untrained") first.
type skillSpec struct {
	Name    string
	Unit    uint32
	Base    uint32
	Labels  []string
	Effects []rankEffect
}

// skillSpecs is every skill's curve, indexed by SkillKind. The steeper the
// curve, the more each rank pays: roughly +10% throughput per rank at base 2,
// +20% at base 3, +30% at base 4 and +50% at base 6, compounded, so a steep
// skill returns more for the same practice. Mining can't yield more ore than
// a tile holds, and foraging can't scrape more scum than a patch has, so they
// only get faster; recipes also yield more. A label may cover more than one
// rank, which keeps a fine curve under fewer names.
var skillSpecs = [numSkills]skillSpec{
	SkillNone: {Name: "none", Labels: []string{""}, Effects: []rankEffect{{100, 100}}},
	SkillMining: {
		Name: "mining", Unit: 60, Base: 2, // rank 1 at 10 tiles
		Labels: []string{"", "rockbreaker", "digger", "digger", "tunneler", "tunneler", "miner", "seasoned miner", "master miner"},
		Effects: []rankEffect{
			{100, 100}, {100, 100}, {100, 100}, {100, 100}, {100, 100}, {100, 100},
			{80, 100}, {80, 100}, {70, 100},
		},
	},
	SkillForaging: {
		Name: "foraging", Unit: 60, Base: 3, // rank 1 at 10 units of scum
		Labels:  []string{"", "gleaner", "scraper", "forager", "seasoned forager", "master forager"},
		Effects: []rankEffect{{100, 100}, {100, 100}, {83, 100}, {69, 100}, {58, 100}, {48, 100}},
	},
	SkillCooking: {
		Name: "cooking", Unit: 120, Base: 4, // rank 1 at about 10 batches
		Labels:  []string{"", "kitchen hand", "cook", "chef", "master chef"},
		Effects: []rankEffect{{100, 100}, {100, 100}, {85, 110}, {70, 120}, {60, 130}},
	},
	SkillConstruction: {
		Name: "construction", Unit: 40, Base: 4, // rank 1 at 5 walls
		Labels:  []string{"", "laborer", "builder", "mason", "master builder"},
		Effects: []rankEffect{{100, 100}, {100, 100}, {77, 100}, {59, 100}, {45, 100}},
	},
	SkillSmithing: {
		Name: "smithing", Unit: 80, Base: 6, // rank 1 at two ingots
		Labels:  []string{"", "apprentice smith", "journeyman smith", "smith", "master smith"},
		Effects: []rankEffect{{100, 100}, {100, 100}, {75, 110}, {55, 125}, {40, 140}},
	},
}

// rankAt is the rank practice reaches on this curve.
func (s *skillSpec) rankAt(practice uint32) int {
	r, threshold := 0, uint64(s.Unit)
	for r+1 < len(s.Labels) && threshold > 0 && uint64(practice) >= threshold {
		r++
		threshold *= uint64(s.Base)
	}
	return r
}

// threshold is the practice rank r needs (0 for rank 0).
func (s *skillSpec) threshold(r int) uint32 {
	if r <= 0 {
		return 0
	}
	t := uint64(s.Unit)
	for i := 1; i < r; i++ {
		t *= uint64(s.Base)
	}
	return uint32(min(t, math.MaxUint32))
}

// rank is e's rank in k.
func (e *Entity) rank(k SkillKind) int {
	if k == SkillNone || k >= numSkills {
		return 0
	}
	return skillSpecs[k].rankAt(e.practice[k])
}

// skillLabel is e's title in k: "" when untrained.
func (e *Entity) skillLabel(k SkillKind) string {
	if k == SkillNone || k >= numSkills {
		return ""
	}
	return skillSpecs[k].Labels[e.rank(k)]
}

// practise credits e with baseTicks of completed work in k: the work's base
// ticks, before e's traits or skill made it faster, so getting faster doesn't
// slow learning. Reaching a rank with a new label is a memory, and so is
// taking up a new trade.
func (w *World) practise(e *Entity, k SkillKind, baseTicks int) {
	if e == nil || e.Kind != Colonist || k == SkillNone || k >= numSkills || baseTicks <= 0 {
		return
	}
	add := uint64(baseTicks) * uint64(max(0, w.cfg.SkillPracticePercent)) / 100
	if add == 0 {
		return
	}
	before := e.skillLabel(k)
	e.practice[k] = uint32(min(uint64(e.practice[k])+add, math.MaxUint32))
	if after := e.skillLabel(k); after != before {
		w.emitDone(e, ActionLearn, NounSkill, "Became %s.", withArticle(after))
	}
	if prev := e.profession; w.updateProfession(e) && prev != SkillNone {
		w.emitDone(e, ActionLearn, NounSkill, "Took up %s as a trade.", e.profession)
	}
}

// updateProfession sets e's profession to the skill it stands highest in,
// where standing is rank as a share of the skill's top rank. Raw ranks don't
// compare across curves: mining's shallow curve has eight ranks to cooking's
// four, so by rank every colonist who mines a little is a miner before it is
// a chef. The profession changes only when another skill stands at least
// level with it even a rank down: a full rank ahead. So a colonist doesn't
// flip between two trades at a threshold;
// ties between new candidates go to SkillKind order. It reports whether the
// profession changed.
func (w *World) updateProfession(e *Entity) bool {
	best := SkillNone
	for k := SkillKind(1); k < numSkills; k++ {
		if r := e.rank(k); r > 0 && (best == SkillNone || standsAbove(r, topRank(k), e.rank(best), topRank(best))) {
			best = k
		}
	}
	if best == SkillNone || best == e.profession {
		return false
	}
	// A rank down, best must still stand at least level with the current trade.
	if cur := e.profession; cur != SkillNone && standsAbove(e.rank(cur), topRank(cur), e.rank(best)-1, topRank(best)) {
		return false
	}
	e.profession = best
	return true
}

// topRank is k's highest rank.
func topRank(k SkillKind) int { return len(skillSpecs[k].Labels) - 1 }

// standsAbove reports whether rank ra of top ta is a larger share than rb of
// top tb, in integers.
func standsAbove(ra, ta, rb, tb int) bool { return ra*tb > rb*ta }

// professionLabel is e's title in its profession: "" when it has none.
func (e *Entity) professionLabel() string { return e.skillLabel(e.profession) }

// ---- Effects ------------------------------------------------------------------

// skillEffect is what e's rank in k does to its work: neutral with skills
// off.
func (w *World) skillEffect(e *Entity, k SkillKind) rankEffect {
	if !w.cfg.Skills || k == SkillNone || k >= numSkills {
		return rankEffect{100, 100}
	}
	return skillSpecs[k].Effects[e.rank(k)]
}

// workTicks is how many ticks e takes for work of base ticks in skill k: the
// base, cut by its skill, then scaled by its traits (workScale). Integer
// percent first, so skill adds no float rounding of its own.
func (w *World) workTicks(e *Entity, k SkillKind, base int) int {
	pct := w.skillEffect(e, k).TicksPct
	if pct != 100 {
		base = atLeast1((base*pct + 50) / 100)
	}
	return scaleTicks(base, e.workScale)
}

// skillYield reports whether this run of a recipe in skill k makes one unit
// more than the recipe says. Yield above 100% is counted, not rolled: each
// run adds YieldPct-100 to e's accumulator, and every 100 is one more unit, so
// yield draws no randomness and is exact over time. It only pays out if
// room says the unit fits; otherwise it waits for a run where it does.
func (w *World) skillYield(e *Entity, k SkillKind, room func() bool) bool {
	extra := w.skillEffect(e, k).YieldPct - 100
	if extra <= 0 {
		return false
	}
	e.yieldAcc[k] += int32(extra)
	if e.yieldAcc[k] < 100 || !room() {
		return false
	}
	e.yieldAcc[k] -= 100
	return true
}

// ---- Backgrounds ----------------------------------------------------------------

// backgroundRoll is one row of the character-generation table: with Chance
// percent, a colonist arrives at rank Rank in one skill, and at Second in
// another (0: none). Rank -1 is the skill's top rank.
type backgroundRoll struct {
	Chance       int
	Rank, Second int
}

var backgroundRolls = []backgroundRoll{
	{Chance: 45, Rank: 1},
	{Chance: 30, Rank: 2},
	{Chance: 15, Rank: 2, Second: 1},
	{Chance: 8, Rank: 3},
	{Chance: 2, Rank: -1},
}

// backgroundWeights is how likely each skill is to be the one a background is
// in. Smithing has half the weight of the others, so a master smith is about
// one colonist in 450.
var backgroundWeights = [numSkills]int{
	SkillMining: 2, SkillForaging: 2, SkillCooking: 2, SkillConstruction: 2, SkillSmithing: 1,
}

// rollBackground gives an arriving colonist the skills of its past, from the
// skill stream: it changes what the colonist does, so it can't be flavor
// (prng), and drawing it from the simulation stream would shift every later
// draw on every seed. Practice is set to the rank's threshold, so a background
// is ordinary practice from then on.
func (w *World) rollBackground(e *Entity) {
	if !w.cfg.Skills || w.skillRNG == nil || e.Kind != Colonist {
		return
	}
	n := w.skillRNG.IntN(100)
	roll := backgroundRolls[len(backgroundRolls)-1]
	for _, r := range backgroundRolls {
		if n < r.Chance {
			roll = r
			break
		}
		n -= r.Chance
	}
	first := w.rollSkill(SkillNone)
	w.setRank(e, first, roll.Rank)
	if roll.Second > 0 {
		w.setRank(e, w.rollSkill(first), roll.Second)
	}
	w.updateProfession(e)
}

// rollSkill picks a skill by backgroundWeights, never skip.
func (w *World) rollSkill(skip SkillKind) SkillKind {
	total := 0
	for k := SkillKind(1); k < numSkills; k++ {
		if k != skip {
			total += backgroundWeights[k]
		}
	}
	n := w.skillRNG.IntN(total)
	for k := SkillKind(1); k < numSkills; k++ {
		if k == skip {
			continue
		}
		if n < backgroundWeights[k] {
			return k
		}
		n -= backgroundWeights[k]
	}
	return SkillNone
}

// setRank raises e's practice in k to rank r's threshold (r < 0: the top
// rank). It never lowers practice.
func (w *World) setRank(e *Entity, k SkillKind, r int) {
	if k == SkillNone || k >= numSkills {
		return
	}
	s := &skillSpecs[k]
	if r < 0 || r >= len(s.Labels) {
		r = len(s.Labels) - 1
	}
	e.practice[k] = max(e.practice[k], s.threshold(r))
}

// ---- Views ---------------------------------------------------------------------

// SkillView is one skill a colonist has some rank in, for a frontend.
type SkillView struct {
	Skill    SkillKind
	Rank     int
	MaxRank  int
	Label    string
	Practice uint32
}

// skillViews is every skill e has a rank in, in SkillKind order.
func (e *Entity) skillViews() []SkillView {
	var out []SkillView
	for k := SkillKind(1); k < numSkills; k++ {
		if r := e.rank(k); r > 0 {
			out = append(out, SkillView{Skill: k, Rank: r, MaxRank: topRank(k),
				Label: skillSpecs[k].Labels[r], Practice: e.practice[k]})
		}
	}
	return slices.Clip(out)
}
