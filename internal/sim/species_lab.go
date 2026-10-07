package sim

// The species-lab surface is the part of the alien roster a tool can call
// without a world: Creature Lab (cmd/creature-lab) rolls one species at a
// time to keep in its catalog and draw sprites for. World.alienSpecies is
// rolled by the same rollAlienSpeciesRoster, so a lab species is exactly the
// species a world with that seed and alien-species-count 1 would roll. See
// docs/creature-lab.md.

// RollLabSpecies rolls the single species a default-config world with this
// seed and one alien species would roll: build, name, scientific name,
// anatomy and lifecycle.
func RollLabSpecies(seed int64) AlienSpecies {
	cfg := DefaultConfig()
	cfg.Seed = seed
	cfg.AlienSpeciesCount = 1
	return rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)[0]
}

// FeaturePhrases names a body's graded features the way the lore tab does:
// "a crown of 9 horns", "hooked claws", "a tail ending in a stinger".
func (a AlienAnatomy) FeaturePhrases() []string { return a.featurePhrases() }

// SizeWords renders a form's size against the adult's in words, as the lore
// tab does ("about a fifth of adult size").
func (f AlienForm) SizeWords() string { return sizeWords(f.SizePct) }
