// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package playground is a PDF editor that runs in a browser tab: a command
// ribbon, the page you are on, and a strip of page thumbnails beside it.
//
// ⛔ NOTHING IS UPLOADED, AND THAT IS A PROPERTY OF THE BUILD rather than a
// promise about our conduct: the whole tool is one WebAssembly module and a
// static page, and there is no endpoint to send a file to.
//
// # What is where
//
//	doc.go        the document and what may be done to it — no widgets here
//	actions.go    one toolkit.Action per command, the single source of truth
//	ribbon.go     the tabs and groups those actions are surfaced in
//	thumbs.go     the thumbnails, drawn when the strip asks and not before
//	scene.go      the widget tree
//
// The split is the MVVM one this fleet uses everywhere a boundary is crossed:
// doc.go knows nothing about go-widgets, and scene.go knows nothing about
// go-pdfkit. A command is reached from the ribbon, from the keyboard and (one
// day) from a palette through the SAME Action, so the three cannot drift.
package playground

import (
	"errors"
	"fmt"

	"github.com/go-pdfkit/ops"
)

// MaxPDF is how large a file may be. The whole document is held in the tab's
// memory, parsed, and held again as pages, so the ceiling is on what arrives
// rather than on what it becomes.
const MaxPDF = 64 << 20

// MaxUndo is how many steps back the editor remembers.
//
// ⛔ Each step is a WHOLE SERIALISED DOCUMENT, because that is what ops works
// in, so this is a memory budget and not a preference: twenty states of a
// forty-megabyte scan is most of a browser tab. Small on purpose.
const MaxUndo = 20

// ErrNoDocument is returned by a command when there is nothing open.
var ErrNoDocument = errors.New("there is no document open")

// Doc is the open document: the bytes, and the page you are on.
//
// It is deliberately not a go-widgets anything. Everything here can be tested
// without a canvas, and is.
type Doc struct {
	bytes []byte
	doc   *ops.Doc
	page  int // 1-based; 0 when nothing is open

	undo [][]byte
	redo [][]byte

	// Name is what the file was called when it arrived, for the Save button.
	Name string
	// Repaired records that ops had to mend the file as it opened, which a
	// reader is told rather than left to wonder about.
	Repaired bool
}

// Open reads b as a PDF and makes it the open document.
func Open(b []byte, name string) (*Doc, error) {
	if len(b) == 0 {
		return nil, errors.New("that file is empty")
	}
	if len(b) > MaxPDF {
		return nil, fmt.Errorf("that file is %d bytes, past the %d this can hold in a tab",
			len(b), MaxPDF)
	}
	d, err := ops.Open(b)
	if err != nil {
		return nil, fmt.Errorf("that file could not be opened as a PDF: %w", err)
	}
	return &Doc{bytes: b, doc: d, page: 1, Name: name, Repaired: d.Repaired()}, nil
}

// PageCount is how many pages the document has, 0 when nothing is open.
func (d *Doc) PageCount() int {
	if d == nil || d.doc == nil {
		return 0
	}
	return d.doc.PageCount()
}

// Page is the page being shown, 1-based.
func (d *Doc) Page() int {
	if d == nil {
		return 0
	}
	return d.page
}

// Bytes is the document as it now stands, for rendering and for saving.
func (d *Doc) Bytes() []byte {
	if d == nil {
		return nil
	}
	return d.bytes
}

// Show moves to page n, clamped to the document.
//
// ⛔ Clamped rather than refused. Turning the page past the end is a thing
// people do with the keyboard held down, and an error for it would be noise.
func (d *Doc) Show(n int) {
	if d == nil || d.PageCount() == 0 {
		return
	}
	switch {
	case n < 1:
		n = 1
	case n > d.PageCount():
		n = d.PageCount()
	}
	d.page = n
}

// Spec is the page range a command applies to.
type Spec string

const (
	// ThisPage is the page being shown. It is resolved at the moment a
	// command runs, not when the ribbon is built.
	ThisPage Spec = ""
	// AllPages is every page.
	AllPages Spec = "all"
)

// resolve turns a Spec into the string ops.ParseRange reads.
func (d *Doc) resolve(s Spec) string {
	if s == ThisPage {
		return fmt.Sprint(d.page)
	}
	return string(s)
}

// Do runs one change on the document and keeps the state before it, so it can
// be undone.
//
// ⛔ THE SNAPSHOT IS TAKEN BEFORE, AND KEPT ONLY IF THE CHANGE SUCCEEDS. A
// failed command that had already pushed an undo state would leave a step that
// undoes nothing, and the second press of Undo would then skip a real one.
func (d *Doc) Do(change func(*ops.Doc) error) error {
	if d == nil || d.doc == nil {
		return ErrNoDocument
	}
	before := d.bytes

	if err := change(d.doc); err != nil {
		// ⛔ The document may have been half-changed before the error. Reopen
		// from the bytes we know, so a refused command leaves nothing behind.
		if reopened, err2 := ops.Open(before); err2 == nil {
			d.doc = reopened
		}
		return err
	}
	out, err := d.doc.Bytes()
	if err != nil {
		if reopened, err2 := ops.Open(before); err2 == nil {
			d.doc = reopened
		}
		return fmt.Errorf("the change could not be written: %w", err)
	}

	d.undo = append(d.undo, before)
	if len(d.undo) > MaxUndo {
		d.undo = d.undo[len(d.undo)-MaxUndo:]
	}
	d.redo = nil
	d.bytes = out
	// The page count may have changed under us — Delete, N-up, Split all do
	// that — so the page being shown is clamped rather than left dangling.
	d.Show(d.page)
	return nil
}

// CanUndo and CanRedo say whether those commands would do anything.
func (d *Doc) CanUndo() bool { return d != nil && len(d.undo) > 0 }
func (d *Doc) CanRedo() bool { return d != nil && len(d.redo) > 0 }

// Undo steps back one change.
func (d *Doc) Undo() error { return d.step(&d.undo, &d.redo) }

// Redo steps forward again.
func (d *Doc) Redo() error { return d.step(&d.redo, &d.undo) }

// step moves one state from `from` onto `to`, which is both Undo and Redo:
// they differ only in which stack is which.
func (d *Doc) step(from, to *[][]byte) error {
	if d == nil || len(*from) == 0 {
		return nil
	}
	n := len(*from) - 1
	prev := (*from)[n]
	*from = (*from)[:n]

	reopened, err := ops.Open(prev)
	if err != nil {
		return fmt.Errorf("that step could not be reopened: %w", err)
	}
	*to = append(*to, d.bytes)
	d.bytes, d.doc = prev, reopened
	d.Show(d.page)
	return nil
}
