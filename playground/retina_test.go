// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

// atRatio lays a scene out for a display of the given pixel ratio and puts the
// toolkit back afterwards.
//
// ⛔ THE METRIC SCALE AND THE FACE ARE PROCESS-WIDE, like toolkit.SetFont. A
// test that set them and walked away would leave every test that ran after it
// measuring a surface twice the size it asked for, and the suite's result
// would depend on its order.
func atRatio(t *testing.T, ratio float64, w, h int) *Scene {
	t.Helper()
	was := toolkit.MetricScale()
	wasFont := toolkit.CurrentFont()
	t.Cleanup(func() {
		toolkit.SetMetricScale(was)
		toolkit.SetFont(wasFont)
	})
	s := NewScene(surfaceW, surfaceH)
	s.SetPixelRatio(ratio)
	s.Resize(w, h)
	return s
}

// TestTheLayoutFollowsTheDisplaysPixelRatio is the test this editor did not
// have, and the defect it did not have it for was that there was nothing to
// see on a Retina display.
//
// ⛔ The bands were raw numbers — a ribbon of 128 pixels, a rail of 150 —
// while the wasm shell called toolkit.SetMetricScale(devicePixelRatio), which
// makes every widget draw twice as large. So the ribbon painted about 256
// pixels of controls into a band told it was 128 and covered the page under
// it. Nothing failed: the layout was wrong only on the machines nobody tests
// on, which is why this asserts the RELATIONS between the bands rather than
// any number.
func TestTheLayoutFollowsTheDisplaysPixelRatio(t *testing.T) {
	// ⛔ MEASURED WHILE THE SCALE IS INSTALLED, not afterwards. The metric
	// scale is one process-wide number: holding two scenes and asking them
	// both at the end asks them both at whichever scale was set last, and the
	// first version of this test reported the ratio-1 scene as 270 pixels tall
	// because the ratio-2 scene had been built in between. It passed the
	// defect it was written to catch — both numbers were equal, which is
	// exactly what a layout that ignores the display looks like.
	one := measure(t, 1)
	two := measure(t, 2)

	if two.ribbon <= one.ribbon {
		t.Errorf("the ribbon is %d pixels tall at ratio 1 and %d at ratio 2: it does not follow the display",
			one.ribbon, two.ribbon)
	}
	if two.rail <= one.rail {
		t.Errorf("the thumbnail rail is %d wide at ratio 1 and %d at ratio 2", one.rail, two.rail)
	}
	if two.status <= one.status {
		t.Errorf("the status line is %d tall at ratio 1 and %d at ratio 2", one.status, two.status)
	}
	if two.text <= one.text {
		t.Errorf("the type is %d pixels tall at ratio 1 and %d at ratio 2: the face did not follow "+
			"the metrics, so the labels rattle around inside controls built for them",
			one.text, two.text)
	}
}

// bands are the editor's own measurements at one pixel ratio.
type bands struct{ ribbon, rail, status, text int }

// measure lays the editor out at a ratio and reads it THERE.
func measure(t *testing.T, ratio float64) bands {
	t.Helper()
	w, h := int(float64(surfaceW)*ratio), int(float64(surfaceH)*ratio)
	s := atRatio(t, ratio, w, h)
	return bands{
		ribbon: s.ribbonHeight(),
		rail:   s.stripRect().W,
		status: statusHeight(),
		text:   toolkit.CurrentFont().Height(),
	}
}

// TestNothingOverlapsTheRibbonAtAnyRatio is the invariant the defect broke.
func TestNothingOverlapsTheRibbonAtAnyRatio(t *testing.T) {
	for _, ratio := range []float64{1, 1.5, 2, 3} {
		s := atRatio(t, ratio, int(float64(surfaceW)*ratio), int(float64(surfaceH)*ratio))
		band := s.ribbonRect()
		if band.H != s.ribbon.Height() {
			t.Errorf("ratio %g: the ribbon is given %d pixels and paints %d",
				ratio, band.H, s.ribbon.Height())
		}
		for name, r := range map[string]struct{ X, Y, W, H int }{
			"the page":  {s.pageRect().X, s.pageRect().Y, s.pageRect().W, s.pageRect().H},
			"the strip": {s.stripRect().X, s.stripRect().Y, s.stripRect().W, s.stripRect().H},
		} {
			if r.Y < band.Y+band.H {
				t.Errorf("ratio %g: %s starts at y=%d and the ribbon ends at %d, so the ribbon covers it",
					ratio, name, r.Y, band.Y+band.H)
			}
			if r.W <= 0 || r.H <= 0 {
				t.Errorf("ratio %g: %s is %dx%d, so there is nothing of it to see", ratio, name, r.W, r.H)
			}
			if r.X+r.W > s.w || r.Y+r.H > s.h-statusHeight() {
				t.Errorf("ratio %g: %s runs from (%d,%d) to (%d,%d) on a %dx%d surface whose status line takes %d",
					ratio, name, r.X, r.Y, r.X+r.W, r.Y+r.H, s.w, s.h, statusHeight())
			}
		}
	}
}

// TestTheEditorIsDrawnAtEveryRatio paints it, because a rectangle that is the
// right shape and a surface that has something on it are different claims.
func TestTheEditorIsDrawnAtEveryRatio(t *testing.T) {
	for _, ratio := range []float64{1, 2} {
		w, h := int(float64(surfaceW)*ratio), int(float64(surfaceH)*ratio)
		s := atRatio(t, ratio, w, h)
		b, err := ExamplePDF()
		if err != nil {
			t.Fatal(err)
		}
		s.Load(b, "example.pdf")
		buf := make([]byte, 4*s.w*s.h)
		s.Draw(buf)

		// The ribbon's own band, and the page's own band, each with ink of
		// their own: the page being drawn cannot stand in for the ribbon.
		band := s.ribbonRect()
		if n := inkIn(buf, s.w, 0, 0, band.W, band.H); n < 500 {
			t.Errorf("ratio %g: the ribbon band has %d dark pixels", ratio, n)
		}
		pr := s.pageRect()
		if n := inkIn(buf, s.w, pr.X, pr.Y, pr.W, pr.H); n < 500 {
			t.Errorf("ratio %g: the page band has %d dark pixels — the document is not on screen",
				ratio, n)
		}
	}
}

// inkIn counts the dark pixels in a rectangle of an RGBA buffer w pixels wide.
func inkIn(buf []byte, w, x0, y0, rw, rh int) int {
	n := 0
	for y := y0; y < y0+rh; y++ {
		for x := x0; x < x0+rw; x++ {
			o := (y*w + x) * 4
			if o+2 >= len(buf) {
				continue
			}
			if int(buf[o])+int(buf[o+1])+int(buf[o+2]) < 3*128 {
				n++
			}
		}
	}
	return n
}

func TestARatioThatIsNotOneIsRefused(t *testing.T) {
	// A browser that reports nothing, or something absurd, leaves the editor
	// at one pixel per pixel rather than at a surface nobody can read.
	for _, bad := range []float64{0, -1} {
		s := atRatio(t, bad, surfaceW, surfaceH)
		if got := toolkit.MetricScale(); got != 1 {
			t.Errorf("a reported ratio of %g installed a scale of %g", bad, got)
		}
		if s.pageRect().W <= 0 {
			t.Errorf("a reported ratio of %g left no page band", bad)
		}
	}
}

func TestTheSurfaceNeverLaysOutSmallerThanItsFloor(t *testing.T) {
	// ⛔ The floor is in DEVICE pixels. A fixed 320 would refuse a window that
	// is 320 CSS pixels wide on a Retina display and lay the editor out at
	// twice the size of the canvas it is painted into — which is the original
	// defect again, in one dimension.
	s := atRatio(t, 2, 100, 100)
	if w, h := s.Size(); w < toolkit.Scaled(320) || h < toolkit.Scaled(240) {
		t.Errorf("a tiny window laid the editor out at %dx%d", w, h)
	}
	big := atRatio(t, 2, 2*surfaceW, 2*surfaceH)
	if w, _ := big.Size(); w != 2*surfaceW {
		t.Errorf("a %d pixel wide canvas was laid out at %d", 2*surfaceW, w)
	}
}
