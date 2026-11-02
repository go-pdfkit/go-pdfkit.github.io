// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"bytes"

	"github.com/go-pdfkit/pdfkit"
)

// ExamplePDF builds the document the playground starts with.
//
// Each page carries as many filled squares as its number, and a rule down the
// left edge.
//
// ⛔ SQUARES AND NOT THE WORD "Page 3". pdfkit sets text in EMBEDDED fonts —
// LoadFont takes the bytes of a real font — and there is no standard-14
// shortcut, so printing a page number would mean carrying a typeface into a
// wasm binary that is already six megabytes. Counting squares needs no font,
// reads at thumbnail size, and the rule makes a rotation obvious, which text
// the same way up in a stamp-sized preview would not.
//
// ⛔ BUILT, NOT EMBEDDED. A committed binary fixture is a file nobody rereads
// and no test can explain; this is the library under test producing its own
// input, so it cannot drift from the writer.
func ExamplePDF() ([]byte, error) {
	const (
		pages  = 8
		w, h   = 595.0, 842.0
		margin = 70.0
		side   = 46.0
		gap    = 14.0
	)
	d := pdfkit.New(pdfkit.Options{Title: "playground example"})
	for n := 1; n <= pages; n++ {
		p := d.AddPage(pdfkit.PageSize{Width: w, Height: h})

		// The rule: down the LEFT edge and only part of the height, so the
		// page has no symmetry a rotation could hide behind.
		p.SetStrokeColor(pdfkit.RGB8(0xb0, 0xb0, 0xb0))
		p.SetLineWidth(4)
		p.MoveTo(margin, h-margin)
		p.LineTo(margin, h/2)
		p.Stroke()

		p.SetFillColor(pdfkit.RGB8(0x1f, 0x3a, 0x5f))
		for i := 0; i < n; i++ {
			x := margin + float64(i%4)*(side+gap)
			y := h - margin - side - float64(i/4)*(side+gap)
			p.Rectangle(pdfkit.Rect{X: x, Y: y, Width: side, Height: side})
			p.Fill()
		}
	}
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
