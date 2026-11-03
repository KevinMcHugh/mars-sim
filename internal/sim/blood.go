package sim

// Blood on colonists' clothes (see docs/blood.md). Purely what the colony
// looks like: nothing reads it but the snapshot. It is exact, with no rolls,
// so it touches neither random stream.

// maxBlood is the most blood a colonist's clothes hold: soaked through.
const maxBlood = 6

// BloodTier buckets a blood level for drawing: 0 clean, 1 flecked, 2
// spattered, 3 soaked.
func BloodTier(blood int) int {
	switch {
	case blood <= 0:
		return 0
	case blood <= 2:
		return 1
	case blood <= 4:
		return 2
	default:
		return 3
	}
}

// bloody adds n to e's blood, up to maxBlood. Colonists only: nothing draws
// any other creature's clothes.
func (e *Entity) bloody(n int) {
	if e.Kind == Colonist && n > 0 {
		e.blood = min(maxBlood, e.blood+n)
		e.bloodFade = 0
	}
}

// splatter bloodies every living colonist within blood-splash-radius of p:
// gore was just made there, and whoever stood close wears some of it.
func (w *World) splatter(p Point) {
	r := w.cfg.BloodSplashRadius
	if r < 0 {
		return
	}
	for _, c := range w.colonistsWithin(p, r, 0) {
		c.bloody(2)
	}
}

// fadeBlood wears one step of blood off e every blood-fade-hours: it dries,
// flakes and gets wiped off as the colonist goes about their day. An
// accumulator per colonist, so the fade is exact and every colonist's clock
// starts when they were last bloodied.
func (w *World) fadeBlood(e *Entity) {
	if e.blood == 0 {
		return
	}
	hours := w.cfg.BloodFadeHours
	if hours <= 0 {
		return
	}
	e.bloodFade++
	if e.bloodFade >= hours*w.cfg.TicksPerHour() {
		e.bloodFade = 0
		e.blood--
	}
}
