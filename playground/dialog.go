// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Dialog sizes. A question with one answer in it does not need to be large,
// and a modal that fills the window hides the document it is about.
const (
	dialogW = 420
	dialogH = 190
)

// AskFor puts a command's question to the reader.
//
// ⛔ ONE AT A TIME, AND THE SECOND ASK IS REFUSED RATHER THAN STACKED. Two
// modals over one another leave the reader answering a question they cannot
// see the top of, and the one underneath has no way to be cancelled.
func (s *Scene) AskFor(c Command) {
	if c.Ask == nil || c.RunWith == nil {
		return
	}
	if s.dialog != nil {
		return
	}

	entry := toolkit.NewEntry(c.Ask.Default)
	entry.Placeholder = c.Ask.Default

	run := func() {
		answer := entry.Text().Get()
		s.dialog, s.entry, s.pending = nil, nil, Command{}
		if err := c.RunWith(s.doc, answer); err != nil {
			s.Say(c.Label + ": " + err.Error())
			return
		}
		s.Say(c.Label + " — " + answer)
		s.Changed()
	}
	cancel := func() {
		s.dialog, s.entry, s.pending = nil, nil, Command{}
		s.Say("")
	}

	d := toolkit.NewDialog(c.Ask.Title, toolkit.NewLabel(c.Ask.Prompt),
		toolkit.NewButton("Cancel", cancel),
		toolkit.NewButton(c.Label, run),
	)
	d.Input = entry
	d.Closable = true
	d.OnClose = cancel

	s.dialog, s.entry, s.pending = d, entry, c
	s.confirm, s.cancel = run, cancel
}

// dialogRect centres the panel on the window.
func (s *Scene) dialogRect() painter.Rect {
	w, h := dialogW, dialogH
	if w > s.w-20 {
		w = s.w - 20
	}
	if h > s.h-20 {
		h = s.h - 20
	}
	return painter.Rect{X: (s.w - w) / 2, Y: (s.h - h) / 2, W: w, H: h}
}

// drawDialog paints the scrim and the panel over everything else.
//
// ⛔ The scrim is not decoration: it is what tells a reader that the ribbon
// behind it is not going to answer them. Without it a modal reads as another
// panel, and the first thing anybody does is click a greyed-out button.
func (s *Scene) drawDialog(p painter.Painter) {
	if s.dialog == nil {
		return
	}
	scrim := s.theme.OnSurface
	scrim.A = toolkit.ModalScrimAlpha
	p.FillRect(painter.Rect{X: 0, Y: 0, W: s.w, H: s.h}, scrim)

	s.dialog.SetBounds(s.dialogRect())
	s.dialog.Draw(p, s.theme)
}

// dialogKey handles the two keys every dialog owes its reader.
//
// ⛔ Enter and Escape, and they are handled HERE rather than left to the
// buttons: a reader who has just typed an answer has their hands on the
// keyboard, and reaching for the pointer to confirm what they typed is the
// difference between a tool that is quick and one that is merely possible.
func (s *Scene) dialogKey(k string) bool {
	switch k {
	case "Enter", "NumpadEnter":
		if s.confirm != nil {
			s.confirm()
		}
		return true
	case "Escape":
		if s.cancel != nil {
			s.cancel()
		}
		return true
	}
	// Everything else is text being typed into the field.
	s.dialog.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: k})
	return true
}
