// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-pdfkit/reader"
)

func opened(t *testing.T) *Doc {
	t.Helper()
	b, err := ExamplePDF()
	if err != nil {
		t.Fatalf("the example document does not build: %v", err)
	}
	d, err := Open(b, "example.pdf")
	if err != nil {
		t.Fatalf("the example document does not open: %v", err)
	}
	return d
}

// TestEveryCommandRuns walks the table itself, so a button that does
// something no test covers cannot exist.
//
// ⛔ AND IT CHECKS THE RESULT IS STILL A PDF. A command that corrupted the
// document would leave bytes behind and "no error", and the editor would show
// a blank page with nothing to say why.
func TestEveryCommandRuns(t *testing.T) {
	for _, c := range Commands {
		d := opened(t)
		before := len(d.Bytes())
		if err := c.Run(d); err != nil {
			t.Errorf("%s: %v", c.ID, err)
			continue
		}
		out := d.Bytes()
		if len(out) == 0 {
			t.Errorf("%s: the document is now empty (was %d bytes)", c.ID, before)
			continue
		}
		rd, err := reader.Open(out)
		if err != nil {
			t.Errorf("%s: the result is not a readable PDF: %v", c.ID, err)
			continue
		}
		if rd.PageCount() == 0 {
			t.Errorf("%s: the result has no pages", c.ID)
		}
	}
}

// TestEveryCommandIsUndoable is the other half: a command you cannot take
// back is one nobody will press twice.
func TestEveryCommandIsUndoable(t *testing.T) {
	for _, c := range Commands {
		d := opened(t)
		before := append([]byte(nil), d.Bytes()...)
		if err := c.Run(d); err != nil {
			t.Errorf("%s: %v", c.ID, err)
			continue
		}
		// ⛔ Some commands legitimately change nothing — "move earlier" on
		// page one is a no-op by design — so an unchanged document is not a
		// failure. What must hold is that IF it changed, it can be undone.
		if bytes.Equal(before, d.Bytes()) {
			continue
		}
		if !d.CanUndo() {
			t.Errorf("%s: changed the document and left nothing to undo", c.ID)
			continue
		}
		if err := d.Undo(); err != nil {
			t.Errorf("%s: undo failed: %v", c.ID, err)
			continue
		}
		if !bytes.Equal(before, d.Bytes()) {
			t.Errorf("%s: undo did not restore the document", c.ID)
		}
	}
}

func TestIdsAreUniqueAndLabelled(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands {
		if c.ID == "" || c.Label == "" || c.Tip == "" {
			t.Errorf("%q: a command needs an id, a label and a tip", c.ID)
		}
		if seen[c.ID] {
			t.Errorf("%q appears twice; a keymap binds on the id", c.ID)
		}
		seen[c.ID] = true
		// ⛔ The tip must not merely repeat the label. A tooltip that says
		// "Rotate right: rotate right" costs a reader a hover and tells them
		// nothing.
		if strings.EqualFold(strings.TrimSuffix(c.Tip, "."), c.Label) {
			t.Errorf("%q: the tip only repeats the label", c.ID)
		}
	}
}

func TestRotateActsOnThePageYouAreOn(t *testing.T) {
	// ⛔ The page is resolved when the command RUNS, not when the ribbon is
	// built. Pinning it at build time would rotate page one for ever.
	d := opened(t)
	d.Show(3)
	rotate, ok := CommandByID("page.rotate.right")
	if !ok {
		t.Fatal("no rotate command")
	}
	if err := rotate.Run(d); err != nil {
		t.Fatal(err)
	}
	rd, err := reader.Open(d.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if rd.PageCount() != 8 {
		t.Fatalf("%d pages, want 8", rd.PageCount())
	}
	// Page 3 turned, page 1 did not.
	back, _ := Open(d.Bytes(), "x")
	if got := rotationOf(t, back, 3); got != 90 {
		t.Errorf("page 3 is at %d degrees, want 90", got)
	}
	if got := rotationOf(t, back, 1); got != 0 {
		t.Errorf("page 1 is at %d degrees, want 0 — the wrong page turned", got)
	}
}

// rotationOf reads a page's rotation. ⛔ ops.Doc.Rotation counts pages from
// ONE, like every other spec in that package; written with a -1 here it read
// the page before the one asked about, and reported 0 for a page that had
// turned.
func rotationOf(t *testing.T, d *Doc, page int) int {
	t.Helper()
	r, err := d.doc.Rotation(page)
	if err != nil {
		t.Fatalf("page %d: %v", page, err)
	}
	return r
}

func TestTurningPagesIsClampedNotRefused(t *testing.T) {
	d := opened(t)
	d.Show(0)
	if d.Page() != 1 {
		t.Errorf("page %d, want 1", d.Page())
	}
	d.Show(999)
	if d.Page() != d.PageCount() {
		t.Errorf("page %d, want %d", d.Page(), d.PageCount())
	}
}

func TestDeletingPagesMovesTheViewBack(t *testing.T) {
	// ⛔ After deleting the last page, the page being shown no longer exists.
	// Leaving it dangling is how an editor shows a blank frame and a page
	// number nobody can reach.
	d := opened(t)
	d.Show(d.PageCount())
	del, _ := CommandByID("page.delete")
	if err := del.Run(d); err != nil {
		t.Fatal(err)
	}
	if d.Page() > d.PageCount() {
		t.Errorf("showing page %d of %d", d.Page(), d.PageCount())
	}
}

func TestUndoAndRedoWalkTheSameLine(t *testing.T) {
	d := opened(t)
	start := append([]byte(nil), d.Bytes()...)

	rev, _ := CommandByID("page.reverse")
	if err := rev.Run(d); err != nil {
		t.Fatal(err)
	}
	reversed := append([]byte(nil), d.Bytes()...)

	if err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Bytes(), start) {
		t.Fatal("undo did not come back to the start")
	}
	if !d.CanRedo() {
		t.Fatal("nothing to redo after an undo")
	}
	if err := d.Redo(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Bytes(), reversed) {
		t.Error("redo did not return to the reversed document")
	}
}

func TestANewChangeForgetsTheRedoTrail(t *testing.T) {
	// Standard editor behaviour, and worth pinning: redoing onto a document
	// that has since been changed another way would apply a step that was
	// never taken from here.
	d := opened(t)
	rev, _ := CommandByID("page.reverse")
	if err := rev.Run(d); err != nil {
		t.Fatal(err)
	}
	if err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if !d.CanRedo() {
		t.Fatal("expected a redo step")
	}
	del, _ := CommandByID("page.delete")
	if err := del.Run(d); err != nil {
		t.Fatal(err)
	}
	if d.CanRedo() {
		t.Error("a new change left the old redo trail in place")
	}
}

func TestUndoIsBounded(t *testing.T) {
	d := opened(t)
	rot, _ := CommandByID("page.rotate.right")
	for i := 0; i < MaxUndo+8; i++ {
		if err := rot.Run(d); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.undo) > MaxUndo {
		t.Errorf("%d undo states kept, past the %d allowed — each is a whole document",
			len(d.undo), MaxUndo)
	}
}

func TestWhatOpeningRefuses(t *testing.T) {
	for _, tc := range []struct {
		why string
		in  []byte
	}{
		{"nothing at all", nil},
		{"not a PDF", []byte("hello")},
		{"past the ceiling", make([]byte, MaxPDF+1)},
	} {
		if _, err := Open(tc.in, "x"); err == nil {
			t.Errorf("%s was accepted", tc.why)
		}
	}
}

func TestCommandsOnNothingAreRefusedNotCrashed(t *testing.T) {
	// The ribbon greys these out, but a keyboard shortcut can still arrive
	// before a document does.
	var d *Doc
	for _, c := range Commands {
		if err := c.Run(d); err == nil {
			t.Errorf("%s ran with no document open", c.ID)
		}
	}
}
