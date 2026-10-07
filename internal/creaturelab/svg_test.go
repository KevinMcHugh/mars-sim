package creaturelab

import (
	"errors"
	"strings"
	"testing"
)

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">
  <defs><radialGradient id="g"><stop offset="0" stop-color="#9c6"/></radialGradient></defs>
  <style>.body { fill: url(#g); }</style>
  <ellipse class="body" cx="64" cy="70" rx="44" ry="38" stroke="#1d1512" stroke-width="6"/>
  <use href="#g"/>
</svg>`

func TestCheckSVGAcceptsAHouseStyleSprite(t *testing.T) {
	svg, warnings, err := CheckSVG("\n" + goodSVG + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if svg != goodSVG {
		t.Error("CheckSVG should only trim the source")
	}
	if len(warnings) != 0 {
		t.Errorf("warnings for a house-style sprite: %v", warnings)
	}
}

// Everything that could run or fetch something from the lab's origin is
// refused outright.
func TestCheckSVGRefusesActiveContent(t *testing.T) {
	cases := map[string]string{
		"script":        `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"handler":       `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`,
		"foreignObject": `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject/></svg>`,
		"image":         `<svg xmlns="http://www.w3.org/2000/svg"><image href="#x"/></svg>`,
		"external href": `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="https://evil.example/"><rect/></a></svg>`,
		"js href":       `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect/></a></svg>`,
		"css url":       `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url(https://evil.example/x)"/></svg>`,
		"css import":    `<svg xmlns="http://www.w3.org/2000/svg"><style>@import "https://evil.example/x.css";</style></svg>`,
		"doctype":       `<!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg"/>`,
		"not svg":       `<html><body/></html>`,
		"two roots":     `<svg xmlns="http://www.w3.org/2000/svg"/><svg xmlns="http://www.w3.org/2000/svg"/>`,
		"trailing text": `<svg xmlns="http://www.w3.org/2000/svg"/>hello`,
		"malformed":     `<svg xmlns="http://www.w3.org/2000/svg"><rect></svg>`,
		"empty":         "  ",
		"huge":          `<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat("<rect/>", MaxSVGBytes/7) + `</svg>`,
	}
	for name, svg := range cases {
		if _, _, err := CheckSVG(svg); !errors.Is(err, ErrBadSVG) {
			t.Errorf("%s: err %v, want ErrBadSVG", name, err)
		}
	}
}

func TestLintWarns(t *testing.T) {
	cases := map[string]string{
		"viewBox": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 64"/>`,
		"size":    `<svg xmlns="http://www.w3.org/2000/svg" width="128" viewBox="0 0 128 128"/>`,
		"text":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><text>hi</text></svg>`,
	}
	for name, svg := range cases {
		if _, warnings, err := CheckSVG(svg); err != nil || len(warnings) != 1 {
			t.Errorf("%s: warnings %v, err %v; want one warning", name, warnings, err)
		}
	}
}
