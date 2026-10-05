package wire

import (
	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// RecruitTopic is the Recruit tab: the recruiter's terms, what the treasury
// holds to pay them, and the set of candidates on offer, if any. See
// docs/recruiting.md.
type RecruitTopic struct {
	Fee      int64 `json:"fee"`      // the recruiter's fee for a set
	Cost     int64 `json:"cost"`     // passage for one hire
	SetSize  int   `json:"setSize"`  // candidates in a set; 0: recruiting is off
	Meals    int   `json:"meals"`    // meals each recruit arrives with
	Treasury int64 `json:"treasury"` // what the colony has to pay with
	// Offer is the set on offer's id (0 for none): a hire names it.
	Offer      int         `json:"offer"`
	Candidates []Candidate `json:"candidates"`
	// Ready is false while the founders' ships are still aloft.
	Ready bool `json:"ready"`
	Sets  int  `json:"sets"`  // sets rolled so far
	Hired int  `json:"hired"` // recruits arrived so far
}

// Candidate is one card: who the candidate will be when it arrives, and what
// it brings with it.
type Candidate struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	// Glyph is the emoji it will be drawn with on the map; Look the same in
	// its own skin and hair (see HelloGlyphs.Looks).
	Glyph           string       `json:"glyph"`
	Look            glyphs.Look  `json:"look,omitempty"`
	Pronouns        string       `json:"pronouns"`
	Orientation     string       `json:"orientation"`
	Age             int          `json:"age"`
	Height          string       `json:"height"`
	HeightCM        int          `json:"heightCm"`
	WeightKG        int          `json:"weightKg"`
	Skin            string       `json:"skin"`
	Hair            string       `json:"hair"`
	Traits          []TraitInfo  `json:"traits"`
	Skills          []SkillLevel `json:"skills"`
	Profession      string       `json:"profession,omitempty"`
	ProfessionLabel string       `json:"professionLabel,omitempty"`
	Savings         int64        `json:"savings"`
}

func recruitTopic(s *sim.Snapshot) RecruitTopic {
	r := s.Recruiting
	t := RecruitTopic{
		Fee: int64(r.Fee), Cost: int64(r.Cost), SetSize: r.SetSize, Meals: r.Meals,
		Treasury: int64(s.Economy.Treasury), Offer: r.Offer, Candidates: make([]Candidate, 0, len(r.Candidates)),
		Ready: r.Ready, Sets: r.Sets, Hired: r.Hired,
	}
	for _, c := range r.Candidates {
		p := c.Profile
		card := Candidate{
			Index: c.Index, Name: p.Name,
			Glyph: glyphs.ForColonist(p), Look: glyphs.ForColonistLook(p),
			Pronouns: p.Gender.Pronouns(), Orientation: p.Orientation.String(), Age: p.Age,
			Height: sim.FormatHeight(p.HeightCM), HeightCM: p.HeightCM, WeightKG: p.WeightKG,
			Skin: p.SkinTone.String(), Hair: p.HairColor.String(),
			Traits: make([]TraitInfo, 0, len(p.Traits)), Skills: make([]SkillLevel, 0, len(c.Skills)),
			Savings: int64(c.Savings),
		}
		for _, tr := range p.Traits {
			card.Traits = append(card.Traits, TraitInfo{Name: tr.Name(), Desc: tr.Desc()})
		}
		for _, sk := range c.Skills {
			card.Skills = append(card.Skills, SkillLevel{Name: sk.Skill.String(), Label: sk.Label, Rank: sk.Rank, MaxRank: sk.MaxRank, Practice: sk.Practice})
		}
		if c.Profession != sim.SkillNone {
			card.Profession, card.ProfessionLabel = c.Profession.String(), c.ProfessionLabel
		}
		t.Candidates = append(t.Candidates, card)
	}
	return t
}
