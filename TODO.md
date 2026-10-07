Things to build/fix as we think of them:
* eventually, colonists should pick nicknames for novel alien species
  * meaning, at lore time, we decide how many martian species humanity has already met, and novel ones get named by the colony
* loregen for guns. manufacturers, clip size, etc.
* I think making it possible to convert a wall into a door will fix a lot of problems with folks getting stuck
  * so would allowing the user to create new doors
* centralize content for labels and free text (esp. actor labels and occurrence text) and add authoring to Scum Lab
* romantic relationships, cohabitation (family trees and spouses already exist at worldgen — see docs/ages-and-family.md)
* interpersonal conflict
* organizational conflict: factions, gangs
* crimes
  * thievery, mugging? Mugging sounds hard to do— determining where to do it, witnesses, etc
  * smuggling, illegal goods, black market
* insurance
* lending
* prediction markets
* sickness
* atmospheric conditions and related: temperature, rain from condensation on the cavern roof, humidity, light levels
* creatures, after the species-and-behaviors work (see docs/species-and-behaviors.md and docs/alien-lifecycles.md)
  * phase 6: animals choose by scoring, not a fixed ladder — run cats, rats, chickens and aliens through the colonist focus arbitration (scored candidates, commitment, switch margin) so a starving rat can outweigh a distant cat, or a queen leaves her nest to defend a brood. Changes gameplay; needs balance runs across many seeds
  * phase 7: species in a data file — load ladders as composition only (behaviors and their parameters, never conditions). Wait until the behavior vocabulary stops changing with every feature
  * venom: a sting that keeps hurting — a status component on the victim, and a death path for damage dealt outside the striker's turn
  * defensive quills: spines are the one feature with no combat effect; an attacker striking a quilled alien takes damage back (today only other aliens strike in melee)
  * breeding for single-form species: most species never reproduce, so worldgen and the director set their numbers for good. Maybe adults bear young that are small adults; maybe they never breed
  * habitats: bind a species to a level (deep dwellers, surface grazers) and let that decide where nests are seeded, where it roams, and whether it will take stairs or shafts. Interacts with levels (#159): queens' nest anchors, brood caps and travel estimates all carry a level now
  * forms on the map: eggs, young and queens all draw with the species' glyph. Give forms their own look (Sprite Designer in Scum Lab), and show apex outside the description. Needs setup first
