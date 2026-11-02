// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"strings"
	"testing"
)

// testHost is a Host with no canvas in it, which is the point: the ribbon is
// built and driven in these tests without a widget tree.
type testHost struct {
	doc   *Doc
	said  []string
	dirty int
	opens int
	saves int
}

func (h *testHost) Doc() *Doc    { return h.doc }
func (h *testHost) Say(s string) { h.said = append(h.said, s) }
func (h *testHost) Changed()     { h.dirty++ }
func (h *testHost) Open()        { h.opens++ }
func (h *testHost) Save()        { h.saves++ }

// TestEveryRibbonButtonNamesARealCommand is the drift guard in one direction.
//
// ⛔ A ribbon entry that names nothing is SILENT: buildRibbon skips it, the
// button simply is not there, and nobody finds out until somebody goes
// looking for a command they remember. A typo in an id would do it.
func TestEveryRibbonButtonNamesARealCommand(t *testing.T) {
	acts := actions(&testHost{})
	for _, id := range ribbonIDs() {
		if _, ok := acts[id]; !ok {
			t.Errorf("the ribbon names %q, which is not a command", id)
		}
	}
}

// TestEveryCommandIsOnTheRibbon is the other direction: a command nobody can
// reach is one nobody can use.
func TestEveryCommandIsOnTheRibbon(t *testing.T) {
	on := map[string]bool{}
	for _, id := range ribbonIDs() {
		on[id] = true
	}
	for _, c := range Commands {
		if !on[c.ID] {
			t.Errorf("%q is a command with no button anywhere on the ribbon", c.ID)
		}
	}
}

func TestTheRibbonBuildsItsTabs(t *testing.T) {
	r := buildRibbon(actions(&testHost{}))
	if len(r.Tabs) != len(tabs) {
		t.Fatalf("%d tabs, want %d", len(r.Tabs), len(tabs))
	}
	for i, tab := range r.Tabs {
		if tab.Label == "" {
			t.Errorf("tab %d has no label", i)
		}
		if len(tab.Groups) == 0 {
			t.Errorf("tab %q has no groups", tab.Label)
		}
		for _, g := range tab.Groups {
			if len(g.Controls) == 0 {
				t.Errorf("group %q on tab %q has no controls — an id did not resolve",
					g.Label, tab.Label)
			}
			// The group's first control is the primary one and reads as
			// primary, which is what the ribbon guidance asks for.
			if !g.Controls[0].Large {
				t.Errorf("group %q does not lead with a large control", g.Label)
			}
			for _, c := range g.Controls {
				if c.Tip == "" {
					t.Errorf("a control in %q has no tooltip", g.Label)
				}
			}
		}
	}
}

func TestPressingAButtonEditsTheDocumentAndSaysSo(t *testing.T) {
	h := &testHost{doc: opened(t)}
	acts := actions(h)
	before := len(h.doc.Bytes())

	acts["page.reverse"].Run()

	if h.dirty == 0 {
		t.Error("the host was not told the document changed, so nothing would repaint")
	}
	if len(h.said) == 0 {
		t.Error("nothing was said about what happened")
	}
	if len(h.doc.Bytes()) == 0 || !h.doc.CanUndo() {
		t.Errorf("the document did not change (was %d bytes)", before)
	}
}

// TestAButtonThatFailsSaysWhyAndChangesNothing covers the path that matters
// most in a GUI: the press that does not work.
func TestAButtonThatFailsSaysWhyAndChangesNothing(t *testing.T) {
	h := &testHost{} // no document
	acts := actions(h)
	acts["page.delete"].Run()

	if h.dirty != 0 {
		t.Error("a failed command asked for a repaint")
	}
	if len(h.said) == 0 {
		t.Fatal("a failed command said nothing at all")
	}
	if !strings.Contains(h.said[0], "Delete page") {
		t.Errorf("the message does not name the command: %q", h.said[0])
	}
}

func TestOpenAndSaveGoToTheHost(t *testing.T) {
	h := &testHost{doc: opened(t)}
	acts := actions(h)
	acts[idOpen].Run()
	acts[idSave].Run()
	if h.opens != 1 || h.saves != 1 {
		t.Errorf("open=%d save=%d, want 1 and 1", h.opens, h.saves)
	}
}

// TestButtonsAreGreyedOutUntilThereIsSomethingToDo pins the enabling rule.
//
// ⛔ Open is the one command that must stay live with nothing open, or the
// editor is a dead end.
func TestButtonsAreGreyedOutUntilThereIsSomethingToDo(t *testing.T) {
	h := &testHost{}
	acts := actions(h)

	setEnabled(acts, nil)
	if !acts[idOpen].Enabled.Get() {
		t.Error("Open is greyed out with nothing open — the editor is a dead end")
	}
	for _, id := range []string{idSave, idUndo, idRedo, "page.delete", "layout.nup2"} {
		if acts[id].Enabled.Get() {
			t.Errorf("%q is live with no document open", id)
		}
	}

	h.doc = opened(t)
	setEnabled(acts, h.doc)
	if !acts["page.delete"].Enabled.Get() || !acts[idSave].Enabled.Get() {
		t.Error("the document is open and the commands are still grey")
	}
	if acts[idUndo].Enabled.Get() {
		t.Error("Undo is live before anything has been done")
	}

	acts["page.reverse"].Run()
	setEnabled(acts, h.doc)
	if !acts[idUndo].Enabled.Get() {
		t.Error("Undo is grey after a change")
	}
	if acts[idRedo].Enabled.Get() {
		t.Error("Redo is live before anything has been undone")
	}
}
