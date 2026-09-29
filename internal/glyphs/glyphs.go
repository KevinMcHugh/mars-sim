// Package glyphs is the map's symbol set and the rules for picking one: which
// emoji stands for a terrain, a rock, a tile with refuse on it, a colonist in
// a given state, an alien of a given species.
//
// It is shared by every frontend that draws the map with emoji: the terminal
// UI (internal/ui/tui), which adds each glyph's cell width and ASCII fallback
// in its own registry, and the browser, which gets the list and a glyph index
// per entity over the wire (internal/wire). Picking a glyph here rather than
// in each frontend is what keeps a colonist looking the same in both: the
// choice depends on things only the engine side has (a colonist's gender, age
// and traits, an alien's rolled species), so the browser could not repeat it.
//
// Every glyph is a single code point with Emoji_Presentation=Yes, because
// terminals only agree about the width of those; internal/ui/tui/glyphs.go
// explains why, and its tests enforce it for everything listed in All.
package glyphs

import "github.com/kevinmchugh/mars-sim/internal/sim"

const (
	Rock        = "\U0001F7E5" // 🟥 unexcavated regolith: Mars is the red planet
	IronRock    = "\U00002B1B" // ⬛ iron-bearing rock
	IceRock     = "\U0001F7E6" // 🟦 water ice-bearing rock
	Uranium     = "\U0001F7E9" // 🟩 uranium-bearing rock (the glow is the warning)
	ClayRock    = "\U0001F7E7" // 🟧 clay-bearing rock
	Floor       = "  "         // open, walkable space
	Wall        = "\U0001F9F1" // 🧱 built wall
	Hull        = "\U00002B1C" // ⬜ crash pod hull: metal, not masonry
	Pod         = "\U0001F96B" // 🥫 nutrient pod (food)
	Toilet      = "\U0001F6BD" // 🚽 toilet (bladder)
	Bed         = "\U0001F6CC" // 🛌 dormitory bunk (sleep)
	Incinerator = "\U0001F525" // 🔥 incinerator: burns refuse hauled to the trash room
	Storage     = "\U0001F9F0" // 🧰 storage container: six colonist inventories
	Scumhouse   = "\U0001F372" // 🍲 scumhouse: biomatter in, slurry out
	Scum        = "\U0001F7E2" // 🟢 cave scum a colonist could scrape
	Forge       = "\U0001F3ED" // 🏭 forge: iron ore in, steel ingots out
	GunBench    = "\U0001F528" // 🔨 gun bench: steel in, assault rifles out

	Colonist = "\U0001F477" // 👷 colonist of unknown age/gender (no profile)
	Fleeing  = "\U0001F631" // 😱 colonist running from an alien
	Talking  = "\U0001F4AC" // 💬 colonist chatting with another
	Alien    = "\U0001F47D" // 👽 subterranean mutant
	Cat      = "\U0001F408" // 🐈 floor predator hunting rats
	Rat      = "\U0001F400" // 🐀 rat: scavenges pods, scum, and the dead
	Stomp    = "\U0001F97E" // 🥾 colonist chasing down a rat to stomp it
	Fighting = "\U0001F52B" // 🔫 armed colonist standing its ground against an alien
	Gore     = "\U0001FA78" // 🩸 a violent death's residue on a tile
	Corpse   = "\U0001F9B4" // 🦴 a body left where something died, waiting to be hauled
	Cleaning = "\U0001F9F9" // 🧹 colonist scrubbing refuse up or feeding the incinerator
	Hauling  = "\U0001F4E6" // 📦 colonist carrying refuse to the incinerator
	Mutant   = "\U0001F9DF" // 🧟 colonist changed by uranium (see docs/mutation.md)

	// Alien species flavor glyphs: the curated set an AlienSpecies.Emoji (see
	// docs/lore.md and internal/sim/alien-names.yaml) can draw from and still
	// reach the map, via alienGlyph. A species' rolled Emoji is arbitrary
	// runtime data — sim knows nothing about glyphs or widths — so only a
	// string that matches one of these vetted, registered symbols is ever
	// drawn on the map; anything else (a custom -alien-names file's own
	// choice, say) falls back to Alien rather than reaching fitGlyph
	// unvetted.
	Lizard       = "\U0001F98E" // 🦎
	Snake        = "\U0001F40D" // 🐍
	Turtle       = "\U0001F422" // 🐢
	TRex         = "\U0001F996" // 🦖
	Sauropod     = "\U0001F995" // 🦕
	Caterpillar  = "\U0001F41B" // 🐛
	Beetle       = "\U0001FAB2" // 🪲
	Ant          = "\U0001F41C" // 🐜
	Cricket      = "\U0001F997" // 🦗
	Scorpion     = "\U0001F982" // 🦂
	Worm         = "\U0001FAB1" // 🪱
	Saucer       = "\U0001F6F8" // 🛸
	Microbe      = "\U0001F9A0" // 🦠
	SpaceInvader = "\U0001F47E" // 👾
	Skull        = "\U0001F480" // 💀 bony hides
	Cockroach    = "\U0001FAB3" // 🪳 chitinous hides
	Snail        = "\U0001F40C" // 🐌 slimy hides
	Frog         = "\U0001F438" // 🐸 slimy and spotted
	Tiger        = "\U0001F405" // 🐅 striped
	TigerFace    = "\U0001F42F" // 🐯 striped
	Zebra        = "\U0001F993" // 🦓 striped, four legs
	Leopard      = "\U0001F406" // 🐆 spotted
	Ladybug      = "\U0001F41E" // 🐞 red, spotted, chitinous
	NewMoonFace  = "\U0001F31A" // 🌚
	Pumpkin      = "\U0001F383" // 🎃
	Rabbit       = "\U0001F407" // 🐇
	Dragon       = "\U0001F409" // 🐉
	Crocodile    = "\U0001F40A" // 🐊
	Horse        = "\U0001F40E" // 🐎
	Elephant     = "\U0001F418" // 🐘
	Octopus      = "\U0001F419" // 🐙
	Koala        = "\U0001F428" // 🐨
	MouseFace    = "\U0001F42D" // 🐭
	RabbitFace   = "\U0001F430" // 🐰
	DragonFace   = "\U0001F432" // 🐲
	Hamster      = "\U0001F439" // 🐹
	Wolf         = "\U0001F43A" // 🐺
	Bear         = "\U0001F43B" // 🐻
	Ghost        = "\U0001F47B" // 👻
	AngryImp     = "\U0001F47F" // 👿
	BlueCircle   = "\U0001F535" // 🔵
	SmilingImp   = "\U0001F608" // 😈
	Unicorn      = "\U0001F984" // 🦄
	Butterfly    = "\U0001F98B" // 🦋
	Rhino        = "\U0001F98F" // 🦏
	Squid        = "\U0001F991" // 🦑
	Badger       = "\U0001F9A1" // 🦡
	Troll        = "\U0001F9CC" // 🧌
	TeddyBear    = "\U0001F9F8" // 🧸
	BlackCircle  = "\U000026AB" // ⚫
	NewMoon      = "\U0001F311" // 🌑
	Bat          = "\U0001F987" // 🦇
	Peacock      = "\U0001F99A" // 🦚

	ManAdult     = "\U0001F468" // 👨 adult man colonist
	WomanAdult   = "\U0001F469" // 👩 adult woman colonist
	PersonAdult  = "\U0001F9D1" // 🧑 adult non-binary colonist
	ManSenior    = "\U0001F474" // 👴 senior man colonist
	WomanSenior  = "\U0001F475" // 👵 senior woman colonist
	PersonSenior = "\U0001F9D3" // 🧓 senior non-binary colonist

	// Mars is the header's planet mark. It is drawn inline rather than on
	// the grid, but it goes through the registry like everything else so the
	// startup probe covers it too.
	Mars = "\U0001F534" // 🔴
)

// All lists every glyph, once each. Frontends index into it: the wire sends it
// in the Hello and each entity's glyph as a position in it (see
// docs/wire-format.md), and the TUI's registry must cover exactly this set.
var All = []string{
	Rock,
	IronRock,
	IceRock,
	Uranium,
	ClayRock,
	Floor,
	Wall,
	Hull,
	Pod,
	Toilet,
	Bed,
	Incinerator,
	Storage,
	Scumhouse,
	Scum,
	Forge,
	GunBench,
	Colonist,
	Fleeing,
	Talking,
	Alien,
	Cat,
	Rat,
	Stomp,
	Fighting,
	Gore,
	Corpse,
	Cleaning,
	Hauling,
	Mutant,
	Lizard,
	Snake,
	Turtle,
	TRex,
	Sauropod,
	Caterpillar,
	Beetle,
	Ant,
	Cricket,
	Scorpion,
	Worm,
	Saucer,
	Microbe,
	SpaceInvader,
	Skull,
	Cockroach,
	Snail,
	Frog,
	Tiger,
	TigerFace,
	Zebra,
	Leopard,
	Ladybug,
	NewMoonFace,
	Pumpkin,
	Rabbit,
	Dragon,
	Crocodile,
	Horse,
	Elephant,
	Octopus,
	Koala,
	MouseFace,
	RabbitFace,
	DragonFace,
	Hamster,
	Wolf,
	Bear,
	Ghost,
	AngryImp,
	BlueCircle,
	SmilingImp,
	Unicorn,
	Butterfly,
	Rhino,
	Squid,
	Badger,
	Troll,
	TeddyBear,
	BlackCircle,
	NewMoon,
	Bat,
	Peacock,
	ManAdult,
	WomanAdult,
	PersonAdult,
	ManSenior,
	WomanSenior,
	PersonSenior,
	Mars,
}

var known = func() map[string]bool {
	m := make(map[string]bool, len(All))
	for _, s := range All {
		m[s] = true
	}
	return m
}()

// Known reports whether symbol is one of All.
func Known(symbol string) bool { return known[symbol] }

// Swatch reports whether a glyph is only a patch of color: the colored squares
// that stand for rock and hull, and the blank floor. A terminal can only draw
// color as a character, so the TUI draws these; a frontend with real colors
// (the browser) draws the color instead and keeps glyphs for pictures.
func Swatch(symbol string) bool {
	switch symbol {
	case Rock, IronRock, IceRock, Uranium, ClayRock, Floor, Hull:
		return true
	}
	return false
}

// seniorAge is the age at which a colonist's default glyph switches from an
// adult to a senior variant.
const seniorAge = 60

// ForColonist picks the default map glyph for a colonist at rest: a base
// figure for their gender identity and age bracket. A colonist without a
// profile falls back to Colonist.
//
// A mutant overrides all of that. What uranium did to them is the most
// important thing about that figure on the map — it is why the colony treats
// them differently — and it is not something a gender/age figure can show.
func ForColonist(p *sim.Profile) string {
	if p == nil {
		return Colonist
	}
	if p.HasTrait(sim.TraitMutant) {
		return Mutant
	}
	senior := p.Age >= seniorAge
	switch p.Gender {
	case sim.GenderMan:
		if senior {
			return ManSenior
		}
		return ManAdult
	case sim.GenderWoman:
		if senior {
			return WomanSenior
		}
		return WomanAdult
	default:
		if senior {
			return PersonSenior
		}
		return PersonAdult
	}
}

// ForTerrain is the glyph for bare terrain; rock is the ordinary red square,
// whatever its composition (see ForTile).
func ForTerrain(t sim.Terrain) string {
	switch t {
	case sim.Floor:
		return Floor
	case sim.Wall:
		return Wall
	case sim.Hull:
		return Hull
	case sim.NutrientPod:
		return Pod
	case sim.Toilet:
		return Toilet
	case sim.Bed:
		return Bed
	case sim.Incinerator:
		return Incinerator
	case sim.Storage:
		return Storage
	case sim.Scumhouse:
		return Scumhouse
	case sim.Forge:
		return Forge
	case sim.GunBench:
		return GunBench
	default:
		return Rock
	}
}

// ForComposition is the glyph for rock of a composition.
func ForComposition(c sim.RockComposition) string {
	switch c {
	case sim.IronBearingRock:
		return IronRock
	case sim.WaterIceBearingRock:
		return IceRock
	case sim.UraniumBearingRock:
		return Uranium
	case sim.ClayBearingRock:
		return ClayRock
	default:
		return Rock
	}
}

// ForTile draws an empty tile: refuse takes priority over bare terrain (and
// over the ore in it), since it is the more notable thing to see there, and a
// body outranks the stains around it — it is what a colonist is coming to haul
// away. Any of a stomp, a bite, or a gunshot can leave gore (see
// docs/combat.md); a body is left by a death nothing ate (see
// docs/sanitation.md).
func ForTile(tile sim.Tile) string {
	if tile.Corpses > 0 {
		return Corpse
	}
	if tile.Gore > 0 {
		return Gore
	}
	if tile.Terrain != sim.Rock {
		return ForTerrain(tile.Terrain)
	}
	return ForComposition(tile.Composition)
}

// ForAlien picks a specific alien's map glyph: its species' rolled emoji (see
// sim.AlienSpecies.Emoji), if it names one of this package's glyphs, or the
// generic Alien otherwise. sim carries Emoji as opaque data — it could be any
// string a -alien-names file supplied — so this is the one place that decides
// whether to trust it: a listed symbol has been vetted (the TUI's tests give
// every one a declared width and an ASCII fallback); anything unrecognized
// draws the same generic alien every species used to. See docs/lore.md.
func ForAlien(sp sim.AlienSpecies) string {
	if sp.Emoji != "" && Known(sp.Emoji) {
		return sp.Emoji
	}
	return Alien
}

// ForEntity is an entity's map glyph. A colonist's shows what it is doing when
// that is the notable thing (fleeing, fighting, hauling), and otherwise who it
// is (see ForColonist).
func ForEntity(e sim.EntityView) string {
	switch e.Kind {
	case sim.Alien:
		return ForAlien(e.AlienSpecies)
	case sim.Cat:
		return Cat
	case sim.Rat:
		return Rat
	case sim.Colonist:
		switch e.State {
		case sim.Fleeing:
			return Fleeing
		case sim.Talking:
			return Talking
		case sim.Stomping:
			return Stomp
		case sim.Fighting:
			return Fighting
		case sim.Cleaning:
			return Cleaning
		case sim.Hauling, sim.Storing:
			return Hauling
		default:
			return ForColonist(e.Profile)
		}
	default:
		return Colonist
	}
}
