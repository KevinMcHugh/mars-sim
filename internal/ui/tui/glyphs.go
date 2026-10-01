package tui

import (
	"sync/atomic"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// tileWidth is how many terminal cells one map tile occupies. Every glyph is
// rendered into exactly this many cells, so a map row is always cols*tileWidth
// cells wide no matter what it contains.
const tileWidth = 2

// How wide a terminal paints an emoji is a property of the terminal, not of
// Unicode: it depends on which Unicode version the terminal's width table came
// from and how it treats emoji presentation. Nothing we can compute from the
// string is authoritative. What we can do is refuse to draw anything whose
// width is *contested*, and then check our assumption against the real terminal
// at startup.
//
// The contested constructs, all of which this package bans (enforced by
// TestGlyphRegistryIsUnambiguous):
//
//   - U+FE0F VARIATION SELECTOR-16. "🛏️" is U+1F6CF + VS16: a terminal that
//     honours the selector paints two cells, one that ignores it paints the
//     text-presentation form in one. Our own dependencies disagree about this
//     today — go-runewidth says one cell, x/ansi and uniseg say two — which is
//     exactly the class of bug that shears the grid.
//   - U+200D ZERO WIDTH JOINER, for multi-person and profession sequences.
//   - U+1F3FB..U+1F3FF skin tone modifiers, which a terminal that does not fuse
//     them paints as a separate coloured square.
//
// Every glyph (internal/glyphs) is therefore a single code point with
// Emoji_Presentation=Yes, the one case terminals agree on: two cells, always.
// Skin tone and hair colour live in the colonist's flavour text instead (see
// renderColonistDetail); they never enter a glyph.

// The symbols themselves, and the rules for picking one, live in
// internal/glyphs, shared with the browser. These aliases keep this package's
// rendering code and tests reading as before; the registry below is what the
// terminal adds: a declared width and an ASCII fallback for each.
const (
	glyphRock         = glyphs.Rock
	glyphIronRock     = glyphs.IronRock
	glyphIceRock      = glyphs.IceRock
	glyphUranium      = glyphs.Uranium
	glyphClayRock     = glyphs.ClayRock
	glyphFloor        = glyphs.Floor
	glyphWall         = glyphs.Wall
	glyphHull         = glyphs.Hull
	glyphPod          = glyphs.Pod
	glyphToilet       = glyphs.Toilet
	glyphBed          = glyphs.Bed
	glyphIncinerator  = glyphs.Incinerator
	glyphStorage      = glyphs.Storage
	glyphScumhouse    = glyphs.Scumhouse
	glyphScum         = glyphs.Scum
	glyphSalt         = glyphs.Salt
	glyphForge        = glyphs.Forge
	glyphGunBench     = glyphs.GunBench
	glyphChair        = glyphs.Chair
	glyphIncubator    = glyphs.Incubator
	glyphColonist     = glyphs.Colonist
	glyphFleeing      = glyphs.Fleeing
	glyphTalking      = glyphs.Talking
	glyphAlien        = glyphs.Alien
	glyphCat          = glyphs.Cat
	glyphRat          = glyphs.Rat
	glyphStomp        = glyphs.Stomp
	glyphFighting     = glyphs.Fighting
	glyphGore         = glyphs.Gore
	glyphCorpse       = glyphs.Corpse
	glyphCleaning     = glyphs.Cleaning
	glyphHauling      = glyphs.Hauling
	glyphMutant       = glyphs.Mutant
	glyphLizard       = glyphs.Lizard
	glyphSnake        = glyphs.Snake
	glyphTurtle       = glyphs.Turtle
	glyphTRex         = glyphs.TRex
	glyphSauropod     = glyphs.Sauropod
	glyphCaterpillar  = glyphs.Caterpillar
	glyphBeetle       = glyphs.Beetle
	glyphAnt          = glyphs.Ant
	glyphCricket      = glyphs.Cricket
	glyphScorpion     = glyphs.Scorpion
	glyphWorm         = glyphs.Worm
	glyphSaucer       = glyphs.Saucer
	glyphMicrobe      = glyphs.Microbe
	glyphSpaceInvader = glyphs.SpaceInvader
	glyphSkull        = glyphs.Skull
	glyphCockroach    = glyphs.Cockroach
	glyphSnail        = glyphs.Snail
	glyphFrog         = glyphs.Frog
	glyphTiger        = glyphs.Tiger
	glyphTigerFace    = glyphs.TigerFace
	glyphZebra        = glyphs.Zebra
	glyphLeopard      = glyphs.Leopard
	glyphLadybug      = glyphs.Ladybug
	glyphNewMoonFace  = glyphs.NewMoonFace
	glyphPumpkin      = glyphs.Pumpkin
	glyphRabbit       = glyphs.Rabbit
	glyphDragon       = glyphs.Dragon
	glyphCrocodile    = glyphs.Crocodile
	glyphHorse        = glyphs.Horse
	glyphElephant     = glyphs.Elephant
	glyphOctopus      = glyphs.Octopus
	glyphKoala        = glyphs.Koala
	glyphMouseFace    = glyphs.MouseFace
	glyphRabbitFace   = glyphs.RabbitFace
	glyphDragonFace   = glyphs.DragonFace
	glyphHamster      = glyphs.Hamster
	glyphWolf         = glyphs.Wolf
	glyphBear         = glyphs.Bear
	glyphGhost        = glyphs.Ghost
	glyphAngryImp     = glyphs.AngryImp
	glyphBlueCircle   = glyphs.BlueCircle
	glyphSmilingImp   = glyphs.SmilingImp
	glyphUnicorn      = glyphs.Unicorn
	glyphButterfly    = glyphs.Butterfly
	glyphRhino        = glyphs.Rhino
	glyphSquid        = glyphs.Squid
	glyphBadger       = glyphs.Badger
	glyphTroll        = glyphs.Troll
	glyphTeddyBear    = glyphs.TeddyBear
	glyphBlackCircle  = glyphs.BlackCircle
	glyphNewMoon      = glyphs.NewMoon
	glyphBat          = glyphs.Bat
	glyphPeacock      = glyphs.Peacock
	glyphManAdult     = glyphs.ManAdult
	glyphWomanAdult   = glyphs.WomanAdult
	glyphPersonAdult  = glyphs.PersonAdult
	glyphManSenior    = glyphs.ManSenior
	glyphWomanSenior  = glyphs.WomanSenior
	glyphPersonSenior = glyphs.PersonSenior
	glyphMars         = glyphs.Mars
)

// glyph is one drawable symbol: what we want to draw, how many cells we claim
// it takes, and what to draw instead on a terminal that disagrees.
//
// The fallback is not decoration. A glyph whose real width differs from cells
// by even one column desyncs the line it is on, and the fallback is pure ASCII,
// whose width no terminal has ever disputed.
type glyph struct {
	symbol   string // the emoji we would like to draw
	cells    int    // how many terminal cells we claim symbol occupies
	fallback string // ASCII stand-in, exactly tileWidth cells
}

// glyphRegistry is every symbol the UI may draw. Rendering goes through it, so
// a glyph added to the code without an entry here fails a test rather than
// quietly corrupting a frame at runtime.
var glyphRegistry = map[string]glyph{
	glyphRock:        {glyphRock, 2, "##"},
	glyphIronRock:    {glyphIronRock, 2, "Fe"},
	glyphIceRock:     {glyphIceRock, 2, "H2"},
	glyphUranium:     {glyphUranium, 2, "U "},
	glyphClayRock:    {glyphClayRock, 2, "Cl"},
	glyphFloor:       {glyphFloor, 2, "  "},
	glyphWall:        {glyphWall, 2, "[]"},
	glyphHull:        {glyphHull, 2, "HH"},
	glyphPod:         {glyphPod, 2, "%%"},
	glyphToilet:      {glyphToilet, 2, "WC"},
	glyphBed:         {glyphBed, 2, "=="},
	glyphIncinerator: {glyphIncinerator, 2, "&&"},
	glyphStorage:     {glyphStorage, 2, "[]"},
	glyphScumhouse:   {glyphScumhouse, 2, "Sh"},
	glyphScum:        {glyphScum, 2, ",,"},
	glyphSalt:        {glyphSalt, 2, "::"},
	glyphForge:       {glyphForge, 2, "Fg"},
	glyphGunBench:    {glyphGunBench, 2, "Gb"},
	glyphChair:       {glyphChair, 2, "Ch"},
	glyphIncubator:   {glyphIncubator, 2, "In"},

	glyphColonist: {glyphColonist, 2, "@ "},
	glyphFleeing:  {glyphFleeing, 2, "@!"},
	glyphTalking:  {glyphTalking, 2, "@?"},
	glyphAlien:    {glyphAlien, 2, "A "},
	glyphCat:      {glyphCat, 2, "f "},
	glyphRat:      {glyphRat, 2, "r "},
	glyphStomp:    {glyphStomp, 2, "@*"},
	glyphFighting: {glyphFighting, 2, "@="},
	glyphGore:     {glyphGore, 2, "~~"},
	glyphCorpse:   {glyphCorpse, 2, "%!"},
	glyphCleaning: {glyphCleaning, 2, "@/"},
	glyphHauling:  {glyphHauling, 2, "@+"},
	glyphMutant:   {glyphMutant, 2, "@%"},

	glyphLizard:       {glyphLizard, 2, "Lz"},
	glyphSnake:        {glyphSnake, 2, "Sn"},
	glyphTurtle:       {glyphTurtle, 2, "Tu"},
	glyphTRex:         {glyphTRex, 2, "Rx"},
	glyphSauropod:     {glyphSauropod, 2, "Sp"},
	glyphCaterpillar:  {glyphCaterpillar, 2, "Bg"},
	glyphBeetle:       {glyphBeetle, 2, "Bt"},
	glyphAnt:          {glyphAnt, 2, "An"},
	glyphCricket:      {glyphCricket, 2, "Cr"},
	glyphScorpion:     {glyphScorpion, 2, "Sc"},
	glyphWorm:         {glyphWorm, 2, "Wm"},
	glyphSaucer:       {glyphSaucer, 2, "UF"},
	glyphMicrobe:      {glyphMicrobe, 2, "Mb"},
	glyphSpaceInvader: {glyphSpaceInvader, 2, "SI"},
	glyphSkull:        {glyphSkull, 2, "Sk"},
	glyphCockroach:    {glyphCockroach, 2, "Rc"},
	glyphSnail:        {glyphSnail, 2, "Sl"},
	glyphFrog:         {glyphFrog, 2, "Fr"},
	glyphTiger:        {glyphTiger, 2, "Tg"},
	glyphTigerFace:    {glyphTigerFace, 2, "Tg"},
	glyphZebra:        {glyphZebra, 2, "Zb"},
	glyphLeopard:      {glyphLeopard, 2, "Lp"},
	glyphLadybug:      {glyphLadybug, 2, "Lb"},
	glyphNewMoonFace:  {glyphNewMoonFace, 2, "Mn"},
	glyphPumpkin:      {glyphPumpkin, 2, "Pk"},
	glyphRabbit:       {glyphRabbit, 2, "Rb"},
	glyphDragon:       {glyphDragon, 2, "Dr"},
	glyphCrocodile:    {glyphCrocodile, 2, "Cc"},
	glyphHorse:        {glyphHorse, 2, "Hr"},
	glyphElephant:     {glyphElephant, 2, "El"},
	glyphOctopus:      {glyphOctopus, 2, "Oc"},
	glyphKoala:        {glyphKoala, 2, "Ko"},
	glyphMouseFace:    {glyphMouseFace, 2, "Ms"},
	glyphRabbitFace:   {glyphRabbitFace, 2, "Rb"},
	glyphDragonFace:   {glyphDragonFace, 2, "Dr"},
	glyphHamster:      {glyphHamster, 2, "Hm"},
	glyphWolf:         {glyphWolf, 2, "Wf"},
	glyphBear:         {glyphBear, 2, "Br"},
	glyphGhost:        {glyphGhost, 2, "Gh"},
	glyphAngryImp:     {glyphAngryImp, 2, "Im"},
	glyphBlueCircle:   {glyphBlueCircle, 2, "Bl"},
	glyphSmilingImp:   {glyphSmilingImp, 2, "Im"},
	glyphUnicorn:      {glyphUnicorn, 2, "Un"},
	glyphButterfly:    {glyphButterfly, 2, "Bf"},
	glyphRhino:        {glyphRhino, 2, "Rh"},
	glyphSquid:        {glyphSquid, 2, "Sq"},
	glyphBadger:       {glyphBadger, 2, "Bd"},
	glyphTroll:        {glyphTroll, 2, "Tl"},
	glyphTeddyBear:    {glyphTeddyBear, 2, "Tb"},
	glyphBlackCircle:  {glyphBlackCircle, 2, "Bk"},
	glyphNewMoon:      {glyphNewMoon, 2, "Nm"},
	glyphBat:          {glyphBat, 2, "Ba"},
	glyphPeacock:      {glyphPeacock, 2, "Pc"},

	glyphManAdult:     {glyphManAdult, 2, "M "},
	glyphWomanAdult:   {glyphWomanAdult, 2, "W "},
	glyphPersonAdult:  {glyphPersonAdult, 2, "P "},
	glyphManSenior:    {glyphManSenior, 2, "m "},
	glyphWomanSenior:  {glyphWomanSenior, 2, "w "},
	glyphPersonSenior: {glyphPersonSenior, 2, "p "},

	glyphMars: {glyphMars, 2, "()"},
}

// renderedGlyphs maps each registered symbol to the string actually written to
// the terminal, pre-fitted to tileWidth cells. It is replaced wholesale by
// useASCIIGlyphs when the startup probe catches the terminal disagreeing with
// the registry, so reads must be atomic: the probe runs on the main goroutine
// before the program starts, but View runs on Bubble Tea's.
var renderedGlyphs atomic.Pointer[map[string]string]

// asciiGlyphs tracks which set renderedGlyphs currently holds, so the UI can
// mention the downgrade instead of leaving the player wondering where the
// emoji went.
var asciiGlyphs atomic.Bool

func init() { renderedGlyphs.Store(buildRenderedGlyphs(false)) }

func buildRenderedGlyphs(ascii bool) *map[string]string {
	out := make(map[string]string, len(glyphRegistry))
	for symbol, g := range glyphRegistry {
		if ascii {
			out[symbol] = cells.Fit(g.fallback, tileWidth)
			continue
		}
		out[symbol] = cells.Fit(g.symbol, tileWidth)
	}
	return &out
}

// useASCIIGlyphs switches the whole UI to the ASCII fallback set. It is all or
// nothing on purpose: a half-emoji, half-ASCII map is harder to read than
// either, and if the terminal got one glyph's width wrong there is no reason to
// trust it about the rest.
func useASCIIGlyphs() {
	renderedGlyphs.Store(buildRenderedGlyphs(true))
	asciiGlyphs.Store(true)
}

// usingASCIIGlyphs reports whether the fallback set is active.
func usingASCIIGlyphs() bool { return asciiGlyphs.Load() }

// fitGlyph returns the drawable form of a symbol: exactly tileWidth cells,
// ASCII if the terminal has been caught disagreeing about emoji widths.
//
// An unregistered symbol still gets fitted rather than dropped — a wrong glyph
// beats a broken grid — but TestGlyphRegistryCoversEveryGlyph makes sure we do
// not ship one.
func fitGlyph(symbol string) string {
	if rendered, ok := (*renderedGlyphs.Load())[symbol]; ok {
		return rendered
	}
	return cells.Fit(symbol, tileWidth)
}

// colonistGlyph, terrainGlyph, tileGlyph, alienGlyph and entityGlyph pick
// through internal/glyphs, so the TUI and the browser agree on every choice;
// see those functions for the rules. The ones that return a fitted string are
// the ones the map draws directly.

func colonistGlyph(p *sim.Profile) string { return glyphs.ForColonist(p) }

func terrainGlyph(t sim.Terrain) string { return fitGlyph(glyphs.ForTerrain(t)) }

func tileGlyph(tile sim.Tile) string { return fitGlyph(glyphs.ForTile(tile)) }

func alienGlyph(sp sim.AlienSpecies) string { return glyphs.ForAlien(sp) }

func entityGlyph(e sim.EntityView) string { return fitGlyph(glyphs.ForEntity(e)) }
