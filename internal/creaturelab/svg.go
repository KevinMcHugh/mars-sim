package creaturelab

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// MaxSVGBytes is the most a sprite may weigh. The house style asks for
// 20-60 lines; anything near this is detail that cannot survive 16px.
const MaxSVGBytes = 64 << 10

// lintSVGBytes is where the size warning starts, the Sprite Designer's.
const lintSVGBytes = 20 << 10

// ErrBadSVG wraps every reason CheckSVG refuses a sprite.
var ErrBadSVG = errors.New("unacceptable svg")

// CheckSVG parses svg and refuses anything that could run or fetch something
// when the file is opened from the lab's own origin: scripts, event handlers,
// <foreignObject>, external references, DTDs. It returns the trimmed SVG and
// the house-style warnings (Lint) for one that passes.
//
// The hard rules are about safety, not taste. The lab serves sprites as
// image/svg+xml from the same origin as its session cookie, and a browser
// navigated straight to an SVG document runs its scripts. The response also
// carries a CSP that forbids them (see the web package), but refusing them
// here keeps them out of the database and out of the game.
func CheckSVG(svg string) (string, []string, error) {
	svg = strings.TrimSpace(svg)
	if svg == "" {
		return "", nil, fmt.Errorf("%w: empty", ErrBadSVG)
	}
	if len(svg) > MaxSVGBytes {
		return "", nil, fmt.Errorf("%w: %d bytes, the limit is %d", ErrBadSVG, len(svg), MaxSVGBytes)
	}
	if err := walkSVG(svg); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrBadSVG, err)
	}
	return svg, Lint(svg), nil
}

// externalURL matches a url(...) that is not a same-document #reference.
var externalURL = regexp.MustCompile(`(?i)url\(\s*['"]?\s*[^#'"\s)]`)

func walkSVG(svg string) error {
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = true
	depth, roots := 0, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return errors.New("DOCTYPE and other <! > directives are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" {
				return fmt.Errorf("processing instruction <?%s?> is not allowed", t.Target)
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return errors.New("text outside the <svg> element")
			}
			if strings.Contains(strings.ToLower(string(t)), "@import") || externalURL.Match(t) {
				return errors.New("style text references an external resource")
			}
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				roots++
				if name != "svg" || roots > 1 {
					return errors.New("the document must be a single <svg> element")
				}
			}
			switch name {
			case "script", "foreignobject", "iframe", "embed", "object", "image":
				return fmt.Errorf("<%s> is not allowed", t.Name.Local)
			}
			for _, a := range t.Attr {
				an := strings.ToLower(a.Name.Local)
				av := strings.ToLower(strings.TrimSpace(a.Value))
				switch {
				case strings.HasPrefix(an, "on"):
					return fmt.Errorf("event handler attribute %s is not allowed", a.Name.Local)
				case an == "href" || an == "src":
					if !strings.HasPrefix(av, "#") {
						return fmt.Errorf("%s=%q points outside the file", a.Name.Local, a.Value)
					}
				case strings.Contains(av, "javascript:"):
					return fmt.Errorf("attribute %s holds a javascript: URL", a.Name.Local)
				case externalURL.MatchString(av) || strings.Contains(av, "@import"):
					return fmt.Errorf("attribute %s references an external resource", a.Name.Local)
				}
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if roots == 0 {
		return errors.New("no <svg> element")
	}
	return nil
}

// squareViewBox matches viewBox="0 0 N N".
var squareViewBox = regexp.MustCompile(`viewBox\s*=\s*["']\s*0\s+0\s+(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s*["']`)

// rootSize matches a width or height attribute on the root <svg> tag.
var rootSize = regexp.MustCompile(`^<svg\b[^>]*\s(width|height)\s*=`)

// Lint is advice about what breaks in the map atlas or at 16px, as in the
// Sprite Designer's lintSVG. A sprite with warnings can still be accepted.
func Lint(svg string) []string {
	var out []string
	root := svg
	if i := strings.Index(root, "<svg"); i >= 0 {
		root = root[i:]
	}
	if m := squareViewBox.FindStringSubmatch(svg); m == nil || m[1] != m[2] {
		out = append(out, "No square viewBox starting at 0 0 (the house style is 0 0 128 128): the atlas cell may crop or stretch it.")
	}
	if rootSize.MatchString(root) {
		out = append(out, "The root <svg> sets width or height; leave both off so it scales to the tile.")
	}
	if strings.Contains(svg, "<text") {
		out = append(out, "Uses <text>, which depends on the player's fonts and is unreadable at 16px.")
	}
	if len(svg) > lintSVGBytes {
		out = append(out, fmt.Sprintf("It is %d KB. Detail that size will not survive 16px.", len(svg)/1000))
	}
	return out
}
