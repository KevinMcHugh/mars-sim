package sim

import "sort"

// Relationships give colonists a family tree and track affinity between them.
//
// Family is generated at spawn from a dedicated RNG stream (World.prng, the same
// stream personalities use) so it never perturbs the simulation's own RNG. The
// ground truth is a small kinship tree of parent and marriage links; the display
// relationships (sibling, aunt/uncle, grandparent, ...) are *derived* from the
// tree so they stay mutually consistent no matter how colonies grow. Ancestors
// and relatives who never joined the colony exist only as phantom tree nodes that
// connect real colonists.
//
// Affinity is a warmth score between two colonists, in [-AffinityMax,
// AffinityMax], shifted when they talk (see the Talking activity in systems.go).
// Talking is mostly a diminishing-returns positive-feedback loop: a conversation
// tends to exacerbate the valence of the pair's existing affinity (friends grow
// closer, rivals drift further apart), with the step shrinking as affinity nears
// the extreme, so talking alone saturates at half of AffinityMax. Nothing
// simulates against affinity yet; it is tracked and displayed only.

// RelationKind is a familial tie between two colonists, named from one
// colonist's point of view ("b is a's RelParent").
type RelationKind uint8

const (
	RelSpouse RelationKind = iota
	RelParent
	RelChild
	RelSibling
	RelGrandparent
	RelGrandchild
	RelAuntUncle
	RelNibling
)

func (r RelationKind) String() string {
	switch r {
	case RelSpouse:
		return "spouse"
	case RelParent:
		return "parent"
	case RelChild:
		return "child"
	case RelSibling:
		return "sibling"
	case RelGrandparent:
		return "grandparent"
	case RelGrandchild:
		return "grandchild"
	case RelAuntUncle:
		return "aunt/uncle"
	case RelNibling:
		return "nibling"
	default:
		return "relative"
	}
}

// Relation is a familial tie to another colonist, from the subject's viewpoint.
type Relation struct {
	Other EntityID
	Kind  RelationKind
}

// Affinity is how warmly a colonist regards another, raised by talking together.
type Affinity struct {
	Other EntityID
	Value int
}

// kinID identifies a person in the colony's family tree. Colonists are kin
// persons linked back to their EntityID; ancestors and relatives who never
// joined the colony are phantom nodes (entity == 0) that connect the tree.
type kinID uint64

// kinPerson is one node in the family tree.
type kinPerson struct {
	parents [2]kinID // 0 = unknown
	spouse  kinID    // 0 = none
	entity  EntityID // 0 = phantom (not a colonist)
}

func (p *kinPerson) hasParent(id kinID) bool {
	return p.parents[0] == id || p.parents[1] == id
}

// addParentSlot records id as a parent in the first free slot, reporting whether
// it fit (a person has at most two parents).
func (p *kinPerson) addParentSlot(id kinID) bool {
	if p.hasParent(id) {
		return true
	}
	for i := range p.parents {
		if p.parents[i] == 0 {
			p.parents[i] = id
			return true
		}
	}
	return false
}

// newKin allocates a tree node, linked to entity (0 for a phantom ancestor).
func (w *World) newKin(entity EntityID) kinID {
	id := w.nextKinID
	w.nextKinID++
	w.kin[id] = &kinPerson{entity: entity}
	return id
}

// ensureParent returns an existing parent of x, creating a phantom one if x has
// none. Used to attach siblings and grandparents onto a shared ancestor.
func (w *World) ensureParent(x kinID) kinID {
	px := w.kin[x]
	for _, par := range px.parents {
		if par != 0 {
			return par
		}
	}
	par := w.newKin(0)
	px.parents[0] = par
	return par
}

// assignKin gives a colonist a tree node and, with FamilyChance, ties it to an
// existing colonist. Uses the personality RNG so it never perturbs the sim.
func (w *World) assignKin(e *Entity) {
	e.kin = w.newKin(e.ID)
	if w.cfg.FamilyChance <= 0 || w.prng.Intn(100) >= w.cfg.FamilyChance {
		return
	}
	if r, ok := w.randomColonistKin(e.ID); ok {
		w.relate(e, r)
	}
}

// randomColonistKin reservoir-samples an existing colonist (other than self) that
// already has a tree node. Iterates in ID order so the draw is deterministic.
func (w *World) randomColonistKin(self EntityID) (*Entity, bool) {
	var chosen *Entity
	k := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e == nil || e.Kind != Colonist || e.ID == self || e.kin == 0 {
			continue
		}
		k++
		if w.prng.Intn(k) == 0 {
			chosen = e
		}
	}
	return chosen, chosen != nil
}

// relate wires a familial tie between new colonist c and existing colonist r. It
// tries relationship kinds in a random order and applies the first that fits, so
// a blocked spouse (incompatible orientation, already married) or a full parent
// slot falls back to another tie rather than failing.
func (w *World) relate(c, r *Entity) {
	kinds := []RelationKind{
		RelSpouse, RelSibling, RelChild, RelParent,
		RelGrandparent, RelGrandchild, RelAuntUncle, RelNibling,
	}
	w.prng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	for _, k := range kinds {
		if w.wireRelation(c, r, k) {
			return
		}
	}
}

// wireRelation attaches c to the tree so that c is r's relation of the given
// kind, creating phantom nodes as needed. It reports whether the tie was applied
// (some ties are not always possible). c is freshly generated, so its own slots
// are empty.
func (w *World) wireRelation(c, r *Entity, kind RelationKind) (ok bool) {
	defer func() {
		if ok {
			w.kinRevision++
		}
	}()
	kc, kr := c.kin, r.kin
	pc, pr := w.kin[kc], w.kin[kr]
	switch kind {
	case RelSpouse:
		if pc.spouse != 0 || pr.spouse != 0 || !spouseCompatible(c.Profile, r.Profile) {
			return false
		}
		// A pairing involving a non-binary colonist isn't resolved by orientation
		// alone (see spouseCompatible); a coin flip decides it instead, so anybody
		// might marry an enby regardless of how they describe their orientation.
		if nonbinaryPairing(c.Profile, r.Profile) && w.prng.Intn(2) == 0 {
			return false
		}
		pc.spouse, pr.spouse = kr, kc
		return true
	case RelChild: // c is r's child: c's parents are r (and r's spouse, if any)
		if !validParent(r.Profile, c.Profile) {
			return false
		}
		pc.addParentSlot(kr)
		if pr.spouse != 0 {
			// A spouse is also a parent only when their age supports that role.
			// Otherwise the explicitly requested parent still gets the tie.
			spouse := w.kin[pr.spouse]
			if spouse.entity == 0 {
				pc.addParentSlot(pr.spouse)
			} else if parent := w.entities[spouse.entity]; parent != nil &&
				validParent(parent.Profile, c.Profile) {
				pc.addParentSlot(pr.spouse)
			}
		}
		return true
	case RelParent: // c is r's parent: add c as a parent of r
		if !validParent(c.Profile, r.Profile) {
			return false
		}
		return pr.addParentSlot(kc)
	case RelSibling: // c is r's sibling: c shares r's parents
		w.ensureParent(kr)
		for _, par := range pr.parents {
			if par != 0 {
				pc.addParentSlot(par)
			}
		}
		return true
	case RelGrandparent: // c is r's grandparent: c is a parent of r's parent
		par := w.ensureParent(kr)
		return w.kin[par].addParentSlot(kc)
	case RelGrandchild: // c is r's grandchild: c's parent is a child of r
		mid := w.newKin(0)
		w.kin[mid].addParentSlot(kr)
		pc.addParentSlot(mid)
		return true
	case RelAuntUncle: // c is r's aunt/uncle: c is a sibling of r's parent
		par := w.ensureParent(kr)
		w.ensureParent(par)
		for _, gp := range w.kin[par].parents {
			if gp != 0 {
				pc.addParentSlot(gp)
			}
		}
		return true
	case RelNibling: // c is r's nibling: c is a child of a sibling of r
		w.ensureParent(kr)
		sib := w.newKin(0)
		for _, par := range pr.parents {
			if par != 0 {
				w.kin[sib].addParentSlot(par)
			}
		}
		pc.addParentSlot(sib)
		return true
	}
	return false
}

// cachedRelations returns the stable display relationships for e. The cache is
// invalidated by kinRevision whenever a new familial link is added.
func (w *World) cachedRelations(e *Entity, children map[kinID][]kinID) []Relation {
	if e.relationRevision != w.kinRevision {
		e.relations = w.relativesOf(e, children)
		e.relationRevision = w.kinRevision
	}
	return e.relations
}

// validParent is tolerant of hand-built profiles with no age. Real colonists
// always have an age, while this keeps tree helpers useful for tools and tests
// that only populate the fields relevant to their scenario.
func validParent(parent, child *Profile) bool {
	if parent == nil || child == nil || parent.Age <= 0 || child.Age <= 0 {
		return true
	}
	return parent.Age-child.Age >= 20
}

// spouseCompatible reports whether two colonists could plausibly marry: each is
// attracted to the other's gender given their orientation. A gender-based
// orientation label doesn't say anything about attraction to a non-binary
// colonist, so any pairing involving one is plausible here (barring
// asexuality) — wireRelation settles it with a coin flip instead of trying to
// read that off orientation.
func spouseCompatible(a, b *Profile) bool {
	if nonbinaryPairing(a, b) {
		return a.Orientation != Asexual && b.Orientation != Asexual
	}
	return attracted(a, b) && attracted(b, a)
}

// attracted reports whether a colonist with profile from could be drawn to one
// with profile to, from its orientation.
func attracted(from, to *Profile) bool {
	switch from.Orientation {
	case Asexual:
		return false
	case Bisexual:
		return true
	case Homosexual:
		return from.Gender == to.Gender
	default: // Heterosexual
		return oppositeBinaryGender(from.Gender, to.Gender)
	}
}

// oppositeBinaryGender reports whether a and b are man/woman in either order.
// Non-binary is not an opposite of anyone, so a heterosexual pairing skips it.
func oppositeBinaryGender(a, b Gender) bool {
	return (a == GenderMan && b == GenderWoman) || (a == GenderWoman && b == GenderMan)
}

// nonbinaryPairing reports whether either profile is non-binary.
func nonbinaryPairing(a, b *Profile) bool {
	return a.Gender == GenderNonbinary || b.Gender == GenderNonbinary
}

// kinChildren builds a child index (parent node -> its children) for one
// snapshot's worth of relationship derivations.
func (w *World) kinChildren() map[kinID][]kinID {
	ch := make(map[kinID][]kinID)
	for id, p := range w.kin {
		for _, par := range p.parents {
			if par != 0 {
				ch[par] = append(ch[par], id)
			}
		}
	}
	return ch
}

func (w *World) cachedKinChildren() map[kinID][]kinID {
	if w.kinChildrenRevision != w.kinRevision {
		w.kinChildrenCache = w.kinChildren()
		w.kinChildrenRevision = w.kinRevision
	}
	return w.kinChildrenCache
}

// relativesOf derives a colonist's familial ties to other colonists by walking
// its local family tree (at most two generations in each direction, plus a
// spouse), so cost is bounded by family size, not colony size. children is the
// shared index from kinChildren. The result is sorted for stable display.
func (w *World) relativesOf(e *Entity, children map[kinID][]kinID) []Relation {
	ka := e.kin
	if ka == 0 {
		return nil
	}
	seen := make(map[kinID]RelationKind)
	// add records the closest relation to a tree node; kinds are visited from
	// closest to furthest, so the first one recorded wins.
	add := func(node kinID, kind RelationKind) {
		if node == 0 || node == ka {
			return
		}
		kp := w.kin[node]
		if kp == nil || kp.entity == 0 { // phantom: not a colonist to display
			return
		}
		if _, ok := seen[node]; !ok {
			seen[node] = kind
		}
	}
	pa := w.kin[ka]
	add(pa.spouse, RelSpouse)

	parents := nonZeroKin(pa.parents)
	for _, p := range parents {
		add(p, RelParent)
	}
	for _, p := range parents { // grandparents
		for _, gp := range nonZeroKin(w.kin[p].parents) {
			add(gp, RelGrandparent)
		}
	}
	kids := children[ka]
	for _, kd := range kids {
		add(kd, RelChild)
	}
	for _, kd := range kids { // grandchildren
		for _, gk := range children[kd] {
			add(gk, RelGrandchild)
		}
	}
	for _, p := range parents { // siblings share a parent
		for _, sib := range children[p] {
			add(sib, RelSibling)
		}
	}
	for _, p := range parents { // aunts/uncles: parent's siblings
		for _, gp := range nonZeroKin(w.kin[p].parents) {
			for _, au := range children[gp] {
				if au != p {
					add(au, RelAuntUncle)
				}
			}
		}
	}
	for _, p := range parents { // niblings: siblings' children
		for _, sib := range children[p] {
			if sib == ka {
				continue
			}
			for _, nb := range children[sib] {
				add(nb, RelNibling)
			}
		}
	}

	out := make([]Relation, 0, len(seen))
	for node, kind := range seen {
		out = append(out, Relation{Other: w.kin[node].entity, Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Other < out[j].Other
	})
	return out
}

// nonZeroKin returns the non-empty parent slots of a node.
func nonZeroKin(a [2]kinID) []kinID {
	var out []kinID
	for _, v := range a {
		if v != 0 {
			out = append(out, v)
		}
	}
	return out
}

// ---- Affinity ----------------------------------------------------------------

// addAffinity raises the mutual affinity between two colonists, clamped to the
// configured range.
func (w *World) addAffinity(a, b EntityID, delta int) {
	if a == b {
		return
	}
	w.bumpAffinity(a, b, delta)
	w.bumpAffinity(b, a, delta)
}

// bumpAffinity adjusts a's stored affinity toward b (one direction), clamped to
// the signed range.
func (w *World) bumpAffinity(a, b EntityID, delta int) {
	m := w.affinity[a]
	if m == nil {
		m = make(map[EntityID]int)
		w.affinity[a] = m
	}
	m[b] = clampInt(m[b]+delta, -w.cfg.AffinityMax, w.cfg.AffinityMax)
}

// affinityBetween returns the (symmetric) affinity between two colonists, 0 if
// they have never interacted.
func (w *World) affinityBetween(a, b EntityID) int {
	if m := w.affinity[a]; m != nil {
		return m[b]
	}
	return 0
}

// affinitiesOf returns a colonist's affinities toward living colonists, strongest
// first, for stable display.
func (w *World) affinitiesOf(id EntityID) []Affinity {
	m := w.affinity[id]
	if len(m) == 0 {
		return nil
	}
	out := make([]Affinity, 0, len(m))
	for other, v := range m {
		if w.entities[other] == nil {
			continue
		}
		out = append(out, Affinity{Other: other, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Other < out[j].Other
	})
	return out
}

// dropAffinity clears a departed colonist's affinity records in both directions.
func (w *World) dropAffinity(id EntityID) {
	for other := range w.affinity[id] {
		if m := w.affinity[other]; m != nil {
			delete(m, id)
		}
	}
	delete(w.affinity, id)
}

// ---- Conversation outcomes ---------------------------------------------------

// talkAffinityCap is the magnitude at which the talking feedback loop saturates:
// half of AffinityMax, so repeated chatting alone can carry affinity to ±half,
// leaving the outer range for stronger forces added later.
func (w *World) talkAffinityCap() int {
	return atLeast1(w.cfg.AffinityMax / 2)
}

// rollTalkQuality draws a conversation's quality in [-100, 100]. It leans toward
// the valence of the pair's existing affinity (the positive-feedback loop) atop
// a mild positive baseline, then adds random spread so any pair can still be
// pleasantly or unpleasantly surprised.
func (w *World) rollTalkQuality(existing int) int {
	limit := w.talkAffinityCap()
	frac := clampInt(existing*100/limit, -100, 100) // valence as a percent of the cap
	mean := w.cfg.TalkQualityBias + w.cfg.TalkQualityValence*frac/100
	if s := w.cfg.TalkQualitySpread; s > 0 {
		mean += w.rng.Intn(2*s+1) - s
	}
	return clampInt(mean, -100, 100)
}

// talkAffinityDelta converts a conversation's quality into an affinity change.
// The change follows quality's sign and scales with its magnitude, but shrinks
// as affinity approaches the cap in the direction it is heading (diminishing
// returns), so talking never carries affinity past ±talkAffinityCap.
func (w *World) talkAffinityDelta(existing, quality int) int {
	if quality == 0 {
		return 0
	}
	limit := w.talkAffinityCap()
	gain := w.cfg.TalkAffinityGain
	if quality > 0 {
		room := limit - existing // distance left to +limit
		if room <= 0 {
			return 0
		}
		if room > limit {
			room = limit // below zero: full step available while climbing out
		}
		d := gain * quality * room / (100 * limit)
		if d < 1 {
			d = 1
		}
		if d > room {
			d = room
		}
		return d
	}
	room := limit + existing // distance left to -limit
	if room <= 0 {
		return 0
	}
	if room > limit {
		room = limit
	}
	d := gain * (-quality) * room / (100 * limit)
	if d < 1 {
		d = 1
	}
	if d > room {
		d = room
	}
	return -d
}

// talkMoodDelta is how much a conversation shifts a participant's mood: a company
// term (how it feels to spend time with the other, from existing affinity) plus
// a conversation term (how the chat itself went, from quality). So a good chat
// with someone disliked still lifts mood, a so-so chat with a friend nets a
// small lift, and only a genuinely bad chat with a friend turns it negative.
func (w *World) talkMoodDelta(quality, existing int) int {
	company := existing * w.cfg.MoodCompanyWeight / atLeast1(w.cfg.AffinityMax)
	conversation := quality * w.cfg.MoodConversationWeight / 100
	return company + conversation
}

// adjustMood shifts a colonist's mood by delta, clamped to the mood range.
func (w *World) adjustMood(e *Entity, delta int) {
	e.mood = clampInt(e.mood+delta, -w.cfg.MoodMax, w.cfg.MoodMax)
}

// noteConversation records one completed conversation and returns any trait
// fatigue it causes. Introverts have a small social capacity per window; every
// conversation beyond it lowers morale. Other traits retain a large default
// capacity and do not incur this penalty.
func (w *World) noteConversation(e *Entity) int {
	if e.socialWindowStart == 0 || w.tick-e.socialWindowStart >= w.cfg.SocialWindowTicks {
		e.socialWindowStart = w.tick
		e.socialTalkCount = 0
	}
	e.socialTalkCount++
	if e.socialTalkCount <= e.socialCapacity {
		return 0
	}
	return -(e.socialTalkCount - e.socialCapacity) * e.socialPenalty
}
