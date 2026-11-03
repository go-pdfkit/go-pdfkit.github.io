// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import "testing"

func TestIsDarkColourReadsWhatABrowserReturns(t *testing.T) {
	for css, want := range map[string]bool{
		// What getComputedStyle actually hands back, in both themes. The two
		// real ones first: landingkit's --bg, light and dark.
		"rgb(255, 255, 255)": false,
		"rgb(11, 14, 20)":    true,
		// The playground's own page, which is a shade lighter.
		"rgb(11, 20, 24)": true,
		// rgba, which Chrome returns when anything is semi-transparent.
		"rgba(11, 14, 20, 0.82)": true,
		"rgba(255, 255, 255, 1)": false,
		// No spaces, which other engines return.
		"rgb(0,0,0)": true,
		// The threshold itself: a mid grey on each side of 128.
		"rgb(127, 127, 127)": true,
		"rgb(128, 128, 128)": false,
		// ⛔ A background nobody set, which the browser returns as three zeroes
		// and no alpha. Reading it as black made an unstyled page the darkest
		// there is — a dark ribbon on a white page, the one way round that
		// cannot be read.
		"rgba(0, 0, 0, 0)": false,
		// A nearly transparent one is still judged: something shows through.
		"rgba(0, 0, 0, 0.01)": true,
		"transparent":         false,
		"":                    false,
		// Things that are not a colour this reads.
		"rgb()":           false,
		"rgb(1, 2)":       false,
		"#0b0e14":         false,
		"rgb(a, b, c)":    false,
		"color(srgb 0 0)": false,
	} {
		if got := IsDarkColour(css); got != want {
			t.Errorf("IsDarkColour(%q) = %v, want %v", css, got, want)
		}
	}
}
