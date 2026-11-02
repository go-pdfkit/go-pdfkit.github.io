// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-pdfkit/ops"
)

// Command is one thing the editor can do to the document.
//
// ⛔ IT IS DECLARED HERE AND NOWHERE ELSE. The ribbon, the keyboard and the
// tests all reach a command through this table, so a button that does
// something no test covers cannot exist: the test iterates the table.
//
// Each Run takes the Doc rather than the ops.Doc so that it can read the page
// being shown — "rotate this page" has to resolve `this` when it runs, not
// when the ribbon is built.
type Command struct {
	// ID is stable and is what a keymap binds.
	ID string
	// Label is what the button says.
	Label string
	// Tip is the sentence under the name in the tooltip. It says what the
	// command DOES, not what it is called again.
	Tip string
	// NeedsDoc is true for everything that acts on a document; the ribbon
	// greys those out until one is open.
	NeedsDoc bool
	// Run does it.
	Run func(d *Doc) error

	// Ask, when set, is a value the command needs before it can run. The
	// command is then performed by RunWith rather than by Run.
	//
	// ⛔ IT IS DATA, NOT A WIDGET. A command that built its own dialog would
	// drag go-widgets into this file and make every command untestable
	// without a canvas. What a command knows is WHAT it needs to be told;
	// how it is asked belongs to whatever is running it — which is also what
	// lets the same command be driven from a test, and one day from a script.
	Ask *Ask
	// RunWith does it with the answer. Set exactly when Ask is.
	RunWith func(d *Doc, answer string) error
}

// Ask describes one value a command needs.
type Ask struct {
	// Title heads the dialog.
	Title string
	// Prompt says what to type, in words rather than in grammar.
	Prompt string
	// Default is what the box starts with, and is what the command does if
	// the reader presses straight through — so it is always something
	// sensible rather than empty.
	Default string
}

// simple wraps a change that only needs the ops.Doc.
func simple(f func(*ops.Doc) error) func(*Doc) error {
	return func(d *Doc) error { return d.Do(f) }
}

// onSpec wraps a change that takes a page range, resolved when it runs.
func onSpec(s Spec, f func(*ops.Doc, string) error) func(*Doc) error {
	return func(d *Doc) error {
		return d.Do(func(o *ops.Doc) error { return f(o, d.resolve(s)) })
	}
}

// Commands is every command the editor offers.
//
// The set is chosen to be what somebody actually reaches for in a PDF editor,
// and every one of them is a method of ops.Doc — nothing here is invented for
// the playground, so what the buttons do is what the library does.
var Commands = []Command{
	// — Pages —
	{
		ID: "page.rotate.right", Label: "Rotate right", NeedsDoc: true,
		Tip: "Turn this page a quarter turn clockwise.",
		Run: onSpec(ThisPage, func(o *ops.Doc, spec string) error { return o.Rotate(spec, 90) }),
	},
	{
		ID: "page.rotate.left", Label: "Rotate left", NeedsDoc: true,
		Tip: "Turn this page a quarter turn anticlockwise.",
		Run: onSpec(ThisPage, func(o *ops.Doc, spec string) error { return o.Rotate(spec, -90) }),
	},
	{
		ID: "page.rotate.all", Label: "Rotate every page", NeedsDoc: true,
		Tip: "Turn the whole document a quarter turn clockwise.",
		Run: onSpec(AllPages, func(o *ops.Doc, spec string) error { return o.Rotate(spec, 90) }),
	},
	{
		ID: "page.delete", Label: "Delete page", NeedsDoc: true,
		Tip: "Remove the page being shown.",
		Run: onSpec(ThisPage, func(o *ops.Doc, spec string) error { return o.Delete(spec) }),
	},
	{
		ID: "page.insert", Label: "Insert blank", NeedsDoc: true,
		Tip: "Put an empty page in after this one.",
		Run: func(d *Doc) error {
			return d.Do(func(o *ops.Doc) error { return o.InsertBlank(d.Page()) })
		},
	},
	{
		ID: "page.move.up", Label: "Move earlier", NeedsDoc: true,
		Tip: "Swap this page with the one before it.",
		// ⛔ THE DOCUMENT IS CHECKED BEFORE THE EDGE. Written the other way
		// round — "already on page one, nothing to do" first — this returned
		// nil with NO DOCUMENT OPEN, because Page() is 0 then and 0 <= 1. A
		// command that reports success on nothing is worse than one that
		// fails: the editor has no reason to tell anybody.
		Run: func(d *Doc) error {
			if d == nil || d.doc == nil {
				return ErrNoDocument
			}
			if d.Page() <= 1 {
				return nil // already first: nothing to do, and not an error
			}
			return d.Do(func(o *ops.Doc) error { return o.Move(d.Page(), d.Page()-1) })
		},
	},
	{
		ID: "page.move.down", Label: "Move later", NeedsDoc: true,
		Tip: "Swap this page with the one after it.",
		Run: func(d *Doc) error {
			if d == nil || d.doc == nil {
				return ErrNoDocument // see "Move earlier": the edge comes second
			}
			if d.Page() >= d.PageCount() {
				return nil
			}
			return d.Do(func(o *ops.Doc) error { return o.Move(d.Page(), d.Page()+1) })
		},
	},
	{
		ID: "page.select", Label: "Keep only…", NeedsDoc: true,
		Tip: "Throw away every page but the ones named.",
		Ask: &Ask{
			Title:   "Keep only these pages",
			Prompt:  "Which pages — 1, 2-4, 1,3-5, all, odd, even",
			Default: "1-2",
		},
		RunWith: func(d *Doc, spec string) error {
			return d.Do(func(o *ops.Doc) error { return o.Select(spec) })
		},
	},
	{
		ID: "page.delete.range", Label: "Delete…", NeedsDoc: true,
		Tip: "Remove the pages named, rather than only the one on screen.",
		Ask: &Ask{
			Title:   "Delete pages",
			Prompt:  "Which pages — 1, 2-4, 1,3-5, odd, even",
			Default: "1",
		},
		RunWith: func(d *Doc, spec string) error {
			return d.Do(func(o *ops.Doc) error { return o.Delete(spec) })
		},
	},
	{
		ID: "page.reverse", Label: "Reverse order", NeedsDoc: true,
		Tip: "Put the last page first and the first page last.",
		Run: simple(func(o *ops.Doc) error { o.Reverse(); return nil }),
	},

	// — Layout —
	{
		ID: "layout.nup2", Label: "Two to a sheet", NeedsDoc: true,
		Tip: "Lay the pages out two to a sheet, as for printing.",
		Run: simple(func(o *ops.Doc) error { return o.NUp(2) }),
	},
	{
		ID: "layout.nup4", Label: "Four to a sheet", NeedsDoc: true,
		Tip: "Lay the pages out four to a sheet.",
		Run: simple(func(o *ops.Doc) error { return o.NUp(4) }),
	},
	{
		ID: "layout.nup", Label: "N to a sheet…", NeedsDoc: true,
		Tip: "Lay the pages out however many to a sheet you ask for.",
		Ask: &Ask{
			Title:   "Pages to a sheet",
			Prompt:  "How many pages on each sheet",
			Default: "2",
		},
		RunWith: func(d *Doc, s string) error {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				// ⛔ The reader's words back, not strconv's. "strconv.Atoi:
				// parsing \"two\": invalid syntax" names a function nobody
				// here called.
				return fmt.Errorf("%q is not a number of pages", s)
			}
			return d.Do(func(o *ops.Doc) error { return o.NUp(n) })
		},
	},
	{
		ID: "layout.booklet", Label: "Booklet", NeedsDoc: true,
		Tip: "Order the pages so that folding the printed stack makes a booklet.",
		Run: simple(func(o *ops.Doc) error { return o.Booklet() }),
	},
	{
		ID: "layout.divide", Label: "Split in two", NeedsDoc: true,
		Tip: "Cut every sheet down the middle into two pages.",
		Run: simple(func(o *ops.Doc) error { return o.Divide(2, 1) }),
	},
	{
		ID: "layout.onepage", Label: "One long page", NeedsDoc: true,
		Tip: "Join every page into a single tall one.",
		Run: simple(func(o *ops.Doc) error { return o.OnePage() }),
	},

	// — Document —
	{
		ID: "doc.watermark", Label: "Watermark", NeedsDoc: true,
		Tip: "Write a word across every page.",
		Ask: &Ask{
			Title:   "Watermark",
			Prompt:  "What to write across every page",
			Default: "DRAFT",
		},
		RunWith: func(d *Doc, text string) error {
			return d.Do(func(o *ops.Doc) error { return o.Watermark(string(AllPages), text) })
		},
	},
	{
		ID: "doc.numbers", Label: "Page numbers", NeedsDoc: true,
		Tip: "Number the pages along the foot.",
		Ask: &Ask{
			Title:   "Page numbers",
			Prompt:  "The format — %d is the page number",
			Default: "%d",
		},
		RunWith: func(d *Doc, format string) error {
			return d.Do(func(o *ops.Doc) error { return o.PageNumbers(string(AllPages), format) })
		},
	},
	{
		ID: "doc.flatten", Label: "Flatten", NeedsDoc: true,
		Tip: "Turn form fields and annotations into ordinary page content.",
		Run: simple(func(o *ops.Doc) error { o.Flatten(); return nil }),
	},
	{
		ID: "doc.sanitize", Label: "Sanitise", NeedsDoc: true,
		Tip: "Strip scripts, embedded files and anything else that can act.",
		Run: simple(func(o *ops.Doc) error { o.Sanitize(); return nil }),
	},
	{
		ID: "doc.compress", Label: "Compress", NeedsDoc: true,
		Tip: "Make the file smaller without changing what it draws.",
		Run: simple(func(o *ops.Doc) error { o.Compress(); return nil }),
	},
	{
		ID: "doc.deskew", Label: "Straighten", NeedsDoc: true,
		Tip: "Measure how far each scanned page leans and turn it back.",
		Run: simple(func(o *ops.Doc) error { _, err := o.Deskew(ops.DeskewOptions{}); return err }),
	},
	{
		ID: "doc.dropblank", Label: "Drop blank pages", NeedsDoc: true,
		Tip: "Remove the pages that have nothing on them.",
		Run: simple(func(o *ops.Doc) error { _, err := o.DropBlank(ops.BlankOptions{}); return err }),
	},
	{
		ID: "doc.annotations", Label: "Remove comments", NeedsDoc: true,
		Tip: "Take out every annotation: notes, highlights, stamps.",
		Run: simple(func(o *ops.Doc) error { o.RemoveAnnotations(); return nil }),
	},
	{
		ID: "doc.clearinfo", Label: "Clear details", NeedsDoc: true,
		Tip: "Forget the author, the producer and the other metadata.",
		Run: simple(func(o *ops.Doc) error { o.ClearInfo(); return nil }),
	},
}

// Perform runs the command, answering its own question with the default when
// it has one.
//
// ⛔ IT IS WHAT A HOST THAT CANNOT ASK DOES, and that is not only tests: a
// keyboard accelerator, a script, a batch run. Every Ask therefore carries a
// Default that is worth doing, rather than an empty box — "watermark with
// nothing" is not a sensible reading of a key press.
func (c Command) Perform(d *Doc) error {
	if c.Ask != nil {
		return c.RunWith(d, c.Ask.Default)
	}
	return c.Run(d)
}

// CommandByID finds one, for the tests and for a keymap.
func CommandByID(id string) (Command, bool) {
	for _, c := range Commands {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}
