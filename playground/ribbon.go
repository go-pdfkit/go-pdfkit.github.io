// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/toolkit"
)

// Host is what the ribbon needs from whatever is running it: the document,
// somewhere to say things, and a way to ask for a repaint.
//
// ⛔ AN INTERFACE AND NOT THE SCENE. The ribbon is built from the command
// table and tested without a canvas; a direct reference to the scene would
// make that impossible and would put the widget tree in every test's way.
type Host interface {
	// Doc is the open document, nil when there is none.
	Doc() *Doc
	// Say puts a line in the status area — what a command did, or why it
	// would not.
	Say(string)
	// Changed tells the host the document has been edited, so the page and
	// the thumbnails are redrawn.
	Changed()
	// Open and Save go out to the browser; they are nil-safe no-ops in tests.
	Open()
	Save()
}

// tabs is the shape of the ribbon: which commands sit in which group of which
// tab, in order. Everything here is an ID from the Commands table, and a test
// asserts that every one of them resolves and that no command is orphaned.
var tabs = []struct {
	label  string
	groups []group
}{
	{"Home", []group{
		{"File", []string{"$open", "$save"}},
		{"Undo", []string{"$undo", "$redo"}},
		{"Page", []string{"page.rotate.left", "page.rotate.right", "page.delete", "page.insert"}},
		{"Order", []string{"page.move.up", "page.move.down", "page.reverse"}},
	}},
	{"Pages", []group{
		{"Turn", []string{"page.rotate.left", "page.rotate.right", "page.rotate.all"}},
		{"Remove", []string{"page.delete", "doc.dropblank"}},
		{"Arrange", []string{"page.move.up", "page.move.down", "page.reverse", "page.insert"}},
	}},
	{"Layout", []group{
		{"Sheets", []string{"layout.nup2", "layout.nup4", "layout.divide"}},
		{"Print", []string{"layout.booklet", "layout.onepage"}},
	}},
	{"Document", []group{
		{"Mark", []string{"doc.watermark", "doc.numbers"}},
		{"Clean", []string{"doc.flatten", "doc.sanitize", "doc.annotations", "doc.clearinfo"}},
		{"Repair", []string{"doc.deskew", "doc.compress"}},
	}},
}

type group struct {
	label string
	ids   []string
}

// hostCommands are the four that are not document edits — they belong to the
// host, not to ops — and are written with a $ so they cannot collide with a
// command id.
const (
	idOpen = "$open"
	idSave = "$save"
	idUndo = "$undo"
	idRedo = "$redo"
)

// actions builds one toolkit.Action per command, bound to h.
//
// ⛔ ONE ACTION PER COMMAND, SHARED BY EVERY SURFACE. Action is the toolkit's
// own single source of truth for a command: the ribbon button, the keyboard
// accelerator and a command palette all point at the same one, so a command
// that is disabled is disabled everywhere at once.
func actions(h Host) map[string]*toolkit.Action {
	out := map[string]*toolkit.Action{}

	add := func(id, label, tip string, needsDoc bool, run func()) {
		a := &toolkit.Action{
			ID:      id,
			Label:   label,
			Enabled: mvvm.NewObservable(!needsDoc),
			Visible: mvvm.NewObservable(true),
		}
		a.Run = run
		out[id] = a
		_ = tip
	}

	add(idOpen, "Open", "Choose a PDF from this machine.", false, h.Open)
	add(idSave, "Save", "Download the document as it now stands.", true, h.Save)
	add(idUndo, "Undo", "Take back the last change.", true, func() {
		if err := h.Doc().Undo(); err != nil {
			h.Say(err.Error())
			return
		}
		h.Changed()
	})
	add(idRedo, "Redo", "Do again what was just taken back.", true, func() {
		if err := h.Doc().Redo(); err != nil {
			h.Say(err.Error())
			return
		}
		h.Changed()
	})

	for _, c := range Commands {
		c := c
		add(c.ID, c.Label, c.Tip, c.NeedsDoc, func() {
			if err := c.Run(h.Doc()); err != nil {
				// ⛔ SAID, NOT SWALLOWED. A ribbon button that does nothing
				// and reports nothing is the worst outcome available: the
				// reader presses it again.
				h.Say(c.Label + ": " + err.Error())
				return
			}
			h.Say(c.Label)
			h.Changed()
		})
	}
	return out
}

// buildRibbon turns the tab table into the widget.
func buildRibbon(acts map[string]*toolkit.Action) *toolkit.Ribbon {
	r := &toolkit.Ribbon{}
	for _, t := range tabs {
		tab := &toolkit.RibbonTab{Label: t.label}
		for gi, g := range t.groups {
			rg := &toolkit.RibbonGroup{
				Label: g.label,
				// The leftmost groups are the ones a reader reaches for most,
				// so they are the last to shrink: Priority rises to the right.
				Priority: gi,
			}
			for ci, id := range g.ids {
				a, ok := acts[id]
				if !ok {
					continue // buildRibbonIDs, tested, is what guarantees this
				}
				rg.Controls = append(rg.Controls, &toolkit.RibbonControl{
					Kind:   toolkit.RibbonButton,
					Action: a,
					Tip:    tipOf(id),
					// The first control of a group gets the large form, which
					// is what the Windows ribbon guidance asks for: the
					// group's primary command reads as primary.
					Large: ci == 0,
				})
			}
			tab.Groups = append(tab.Groups, rg)
		}
		r.Tabs = append(r.Tabs, tab)
	}
	return r
}

// tipOf is the sentence under a control's name in its tooltip.
func tipOf(id string) string {
	switch id {
	case idOpen:
		return "Choose a PDF from this machine. It is read in this tab and never uploaded."
	case idSave:
		return "Download the document as it now stands."
	case idUndo:
		return "Take back the last change."
	case idRedo:
		return "Do again what was just taken back."
	}
	if c, ok := CommandByID(id); ok {
		return c.Tip
	}
	return ""
}

// ribbonIDs is every id the ribbon names, for the test that checks they all
// resolve and that no command was left off the ribbon by accident.
func ribbonIDs() []string {
	var out []string
	for _, t := range tabs {
		for _, g := range t.groups {
			out = append(out, g.ids...)
		}
	}
	return out
}

// setEnabled updates every action for the document now open.
//
// It is called after each change rather than on a timer: an Enabled that
// lagged a change by a frame is a button that can be pressed once more after
// it should have greyed out.
func setEnabled(acts map[string]*toolkit.Action, d *Doc) {
	has := d != nil && d.PageCount() > 0
	for id, a := range acts {
		switch id {
		case idOpen:
			a.Enabled.Set(true)
		case idUndo:
			a.Enabled.Set(d.CanUndo())
		case idRedo:
			a.Enabled.Set(d.CanRedo())
		default:
			a.Enabled.Set(has)
		}
	}
}
