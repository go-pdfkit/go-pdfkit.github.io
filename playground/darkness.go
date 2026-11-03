// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"strconv"
	"strings"
)

// IsDarkColour reads a CSS "rgb(r, g, b)" or "rgba(r, g, b, a)" and says
// whether it is closer to ink than to paper.
//
// ⛔ THE DECISION LIVES HERE AND THE PLUMBING LIVES IN THE COMMAND. Asking the
// browser for the page's computed background needs syscall/js and can only be
// built for js/wasm, which is why this was written inside the wasm main and
// went untested: there is no host build to run a test against. The part that
// can be wrong on its own is a string and a threshold, so it is a function of
// a string and lives in the package the tests already cover.
//
// Anything it cannot read is light, which is the safer default: dark widgets
// on a light page are much harder to read than the other way round. A colour
// with fewer than three channels is not a colour it read — "rgb()" and
// "transparent" both end up here.
//
// ⛔ AND NEITHER IS A FULLY TRANSPARENT ONE. "rgba(0, 0, 0, 0)" is what the
// browser returns for a background nobody set, and reading its three zeroes as
// black made an unstyled page the darkest there is: the canvas would have gone
// dark on a white page, which is the one way round that cannot be read. The
// alpha is not used to judge the colour — a page is as dark as what shows
// through — but a zero alpha means there is nothing to judge.
func IsDarkColour(css string) bool {
	i := strings.IndexByte(css, '(')
	j := strings.IndexByte(css, ')')
	if i < 0 || j < i {
		return false
	}
	fields := strings.Split(css[i+1:j], ",")
	if len(fields) > 3 {
		// A fourth field is the alpha, and only a zero one is decisive. One
		// this cannot parse says nothing either way, so the colour is judged.
		if a, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64); err == nil && a == 0 {
			return false
		}
		fields = fields[:3]
	}
	if len(fields) != 3 {
		return false
	}
	var sum int
	for _, f := range fields {
		v, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return false
		}
		sum += v
	}
	return sum < 3*128
}
