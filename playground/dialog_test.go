// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"testing"
)

func loaded(t *testing.T) *Scene {
	t.Helper()
	s := NewScene(surfaceW, surfaceH)
	b, err := ExamplePDF()
	if err != nil {
		t.Fatal(err)
	}
	s.Load(b, "example.pdf")
	return s
}

func TestACommandThatNeedsTellingOpensADialog(t *testing.T) {
	s := loaded(t)
	before := len(s.doc.Bytes())

	s.acts["doc.watermark"].Run()

	if s.dialog == nil {
		t.Fatal("no dialog opened")
	}
	// ⛔ And nothing has happened to the document yet. A command that acted
	// and then asked would make Cancel a lie.
	if len(s.doc.Bytes()) != before || s.doc.CanUndo() {
		t.Error("the document was changed before the question was answered")
	}
	if s.entry.Text().Get() != "DRAFT" {
		t.Errorf("the box starts with %q, want the default", s.entry.Text().Get())
	}
}

func TestAnsweringRunsTheCommandWithTheAnswer(t *testing.T) {
	s := loaded(t)
	s.acts["doc.watermark"].Run()
	s.entry.Text().Set("CONFIDENTIAL")
	s.KeyDown("Enter")

	if s.dialog != nil {
		t.Error("the dialog is still open after Enter")
	}
	if !s.doc.CanUndo() {
		t.Fatal("the document was not changed")
	}
	if !contains(s.status, "CONFIDENTIAL") {
		t.Errorf("the status does not report what was done: %q", s.status)
	}
}

func TestEscapeCancelsAndChangesNothing(t *testing.T) {
	s := loaded(t)
	before := len(s.doc.Bytes())
	s.acts["page.select"].Run()
	s.KeyDown("Escape")

	if s.dialog != nil {
		t.Error("Escape left the dialog open")
	}
	if s.doc.CanUndo() || len(s.doc.Bytes()) != before {
		t.Error("Cancel changed the document")
	}
}

// TestAnAnswerThatWillNotDoSaysSoInTheReadersWords covers the path a dialog
// exists to make possible: a reply that is not usable.
func TestAnAnswerThatWillNotDoSaysSoInTheReadersWords(t *testing.T) {
	s := loaded(t)
	s.acts["layout.nup"].Run()
	s.entry.Text().Set("two")
	s.KeyDown("Enter")

	if s.doc.CanUndo() {
		t.Error("an unreadable answer changed the document")
	}
	if !contains(s.status, "\"two\"") {
		t.Errorf("the message does not quote what was typed: %q", s.status)
	}
	// ⛔ And it must not read as a Go error. strconv's own words name a
	// function nobody here called.
	if contains(s.status, "strconv") {
		t.Errorf("the message is strconv's, not ours: %q", s.status)
	}
}

// TestTheDialogTakesEveryEvent is what makes it modal.
func TestTheDialogTakesEveryEvent(t *testing.T) {
	s := loaded(t)
	page := s.doc.Page()
	s.acts["doc.watermark"].Run()

	// A key that would turn the page goes into the field instead.
	s.KeyDown("ArrowRight")
	if s.doc.Page() != page {
		t.Error("the page turned underneath an open dialog")
	}
	// And a click does not reach the ribbon.
	s.Click(10, 10)
	if s.dialog == nil {
		t.Error("a click outside the panel dismissed the dialog")
	}
}

func TestOnlyOneDialogAtATime(t *testing.T) {
	// ⛔ Two modals over one another leave the reader answering a question
	// they cannot see the top of, and the one underneath cannot be cancelled.
	s := loaded(t)
	s.acts["doc.watermark"].Run()
	first := s.dialog
	s.acts["page.select"].Run()
	if s.dialog != first {
		t.Error("a second dialog replaced the one being answered")
	}
}

func TestTheDialogIsDrawnOverEverything(t *testing.T) {
	s := loaded(t)
	plain := paint(t, s)
	s.acts["doc.watermark"].Run()
	withDialog := paint(t, s)

	if len(plain) != len(withDialog) {
		t.Fatal("different buffer sizes")
	}
	same := 0
	for i := range plain {
		if plain[i] == withDialog[i] {
			same++
		}
	}
	// The scrim alone changes most of the surface; an unchanged picture means
	// the panel was never painted.
	if same == len(plain) {
		t.Error("opening a dialog changed nothing on screen")
	}
}

func TestEveryDialogCommandCanBeRunWithoutOne(t *testing.T) {
	// A host with no way to ask — a script, a keyboard accelerator — uses the
	// default, and it has to work.
	for _, c := range Commands {
		if c.Ask == nil {
			continue
		}
		d := opened(t)
		if err := c.Perform(d); err != nil {
			t.Errorf("%s with its default %q: %v", c.ID, c.Ask.Default, err)
		}
	}
}
