// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"testing"
)

// paint draws the scene and returns the buffer, so a test can look at what a
// reader would see.
func paint(t *testing.T, s *Scene) []byte {
	t.Helper()
	w, h := s.Size()
	buf := make([]byte, w*h*4)
	s.Draw(buf)
	return buf
}

// ink counts the pixels that are not the background, which is the cheapest
// honest answer to "is there anything there".
func ink(buf []byte, w, h int) int {
	n := 0
	for i := 0; i+3 < len(buf); i += 4 {
		// Anything appreciably darker than paper counts.
		if int(buf[i])+int(buf[i+1])+int(buf[i+2]) < 3*200 {
			n++
		}
	}
	return n
}

// TestTheRibbonIsActuallyDrawn is the test that a widget tree which compiles
// does not give you.
//
// ⛔ A SCENE THAT LAYS OUT AND PAINTS NOTHING COMPILES PERFECTLY. Every
// SetBounds can be right, every Draw called, and the window still come out
// blank — a theme with no contrast, a zero-height rectangle, a widget never
// given a size. So this paints and counts.
func TestTheRibbonIsActuallyDrawn(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	buf := paint(t, s)

	// Only the ribbon's own band, so ink elsewhere cannot stand in for it.
	band := make([]byte, 0, surfaceW*ribbonH*4)
	band = append(band, buf[:surfaceW*ribbonH*4]...)
	if n := ink(band, surfaceW, ribbonH); n < 500 {
		t.Errorf("the ribbon band has %d dark pixels — it is blank", n)
	}
}

func TestTheStatusLineSaysSomethingBeforeAnythingIsOpen(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	if s.status == "" {
		t.Fatal("the editor opens with nothing to say")
	}
	// ⛔ And the first thing it says is the thing a reader most wants to know
	// about a tool they have just handed a document to.
	if !contains(s.status, "never uploaded") {
		t.Errorf("the opening line does not say the file stays here: %q", s.status)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestOpeningADocumentFillsThePageAndTheStrip is the end-to-end of the model
// and the view, with no browser in it.
func TestOpeningADocumentFillsThePageAndTheStrip(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	b, err := ExamplePDF()
	if err != nil {
		t.Fatal(err)
	}
	s.Load(b, "example.pdf")

	if s.doc == nil || s.doc.PageCount() != 8 {
		t.Fatalf("the document did not open: %+v", s.doc)
	}
	if s.strip.Count() != 8 {
		t.Errorf("the strip shows %d pages, the document has 8", s.strip.Count())
	}
	if s.page == nil {
		t.Fatal("no page was drawn")
	}

	// The strip asks for the thumbnails it can see; Tick makes them.
	if len(s.want) == 0 {
		// Nothing visible yet is legitimate before the first draw, so draw.
		paint(t, s)
	}
	paint(t, s)
	for i := 0; i < 8 && len(s.want) > 0; i++ {
		s.Tick()
	}
	if s.strip.Resident() == 0 {
		t.Error("no thumbnail was ever made, so the strip is a column of placeholders")
	}
}

// TestTurningPagesMovesTheStripWithIt: the page view and the strip are two
// pictures of one thing, and an editor where they disagree is one nobody can
// trust.
func TestTurningPagesMovesTheStripWithIt(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	b, _ := ExamplePDF()
	s.Load(b, "x.pdf")

	s.KeyDown("ArrowRight")
	if s.doc.Page() != 2 {
		t.Fatalf("page %d after one press, want 2", s.doc.Page())
	}
	if got := s.strip.Current().Get(); got != 1 {
		t.Errorf("the strip is on card %d, the view is on page 2", got)
	}

	s.KeyDown("End")
	if s.doc.Page() != 8 || s.strip.Current().Get() != 7 {
		t.Errorf("End went to page %d, card %d", s.doc.Page(), s.strip.Current().Get())
	}

	// ⛔ And past the end does not move: clamped, not wrapped, and certainly
	// not an error dialog.
	s.KeyDown("ArrowRight")
	if s.doc.Page() != 8 {
		t.Errorf("page %d after pressing past the end", s.doc.Page())
	}
}

func TestAnEditRedrawsEverything(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	b, _ := ExamplePDF()
	s.Load(b, "x.pdf")
	paint(t, s)
	for i := 0; i < 8; i++ {
		s.Tick()
	}

	s.acts["page.delete"].Run()

	if s.strip.Count() != 7 {
		t.Errorf("the strip still shows %d cards after a page was deleted", s.strip.Count())
	}
	// ⛔ Every thumbnail is now suspect, not just the deleted one: the pages
	// after it have all shifted along.
	if len(s.want) != 0 {
		t.Errorf("%d stale thumbnail requests survived the edit", len(s.want))
	}
}

func TestResizingRedrawsThePageToTheNewPane(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	b, _ := ExamplePDF()
	s.Load(b, "x.pdf")
	if s.page == nil {
		t.Fatal("no page")
	}
	s.Resize(1600, 1000)
	if s.page == nil {
		t.Error("the page vanished on resize")
	}
	w, h := s.Size()
	if w != 1600 || h != 1000 {
		t.Errorf("the scene is %dx%d after a resize to 1600x1000", w, h)
	}
	// ⛔ A window too small to lay out is clamped rather than refused: a
	// browser reports 0x0 during some layout passes.
	s.Resize(0, 0)
	if w, h := s.Size(); w < 320 || h < 240 {
		t.Errorf("a 0x0 resize left the scene at %dx%d", w, h)
	}
}

func TestLoadingSomethingThatIsNotAPDFSaysSoAndKeepsWhatWasOpen(t *testing.T) {
	s := NewScene(surfaceW, surfaceH)
	b, _ := ExamplePDF()
	s.Load(b, "good.pdf")
	pages := s.doc.PageCount()

	s.Load([]byte("not a pdf"), "bad.pdf")

	if s.doc == nil || s.doc.PageCount() != pages {
		t.Error("a bad file replaced the document that was open")
	}
	if !contains(s.status, "could not be opened") {
		t.Errorf("nothing useful was said: %q", s.status)
	}
}
