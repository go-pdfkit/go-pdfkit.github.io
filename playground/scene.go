// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"fmt"
	"time"

	"github.com/go-opentype/fonts/ibmplexsans"
	"github.com/go-pdfkit/reader"
	"github.com/go-pdfkit/render"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The surface the scene is laid out on before the page says how big it is, in
// LOGICAL pixels — one CSS pixel each.
const (
	surfaceW = 1100
	surfaceH = 760
)

// ⛔ EVERY LENGTH BELOW IS LOGICAL AND GOES THROUGH toolkit.Scaled. On a
// Retina display the toolkit's metric scale is 2, so every widget draws twice
// as large; a band whose height is a raw 128 is then a quarter of the room its
// contents need, and what a reader sees is a ribbon spilling over a page that
// has been pushed off the bottom. That is exactly what this editor did, and
// the reason it is written down here is that nothing fails: the layout is
// wrong only on the machines nobody tests on.
//
// railW is the width of the thumbnail strip, thumbW the width a thumbnail is
// drawn at, and thumbDPI the resolution that makes a page that wide.
//
// ⛔ THE THUMBNAIL IS DRAWN AT THE SIZE IT IS SHOWN. PageStrip scales what it
// is given with nearest-neighbour, so handing it a big bitmap does not buy a
// sharper card — it buys a blurrier one and the memory to go with it. At a
// ratio of 2 the card IS twice as wide in real pixels, so the resolution
// follows it; that is not the same mistake.
const (
	railW    = 150
	thumbW   = 118
	thumbDPI = 14 // ~118 px across an A4 page
	statusH  = 26
	// gutter is the space between the page and what is around it, and pad the
	// inset of the status line's text.
	gutter = 12
	pad    = 10
)

// Scaled lengths. Functions, not constants: the metric scale is installed when
// the page says what display it is on, which is after this file is compiled
// and after the package's vars are initialised.
func railWidth() int  { return toolkit.Scaled(railW) }
func thumbWidth() int { return toolkit.Scaled(thumbW) }
func statusHeight() int {
	// The status line holds one line of text, so it follows the face like
	// every other band — a fixed 26 clips the moment the type grows.
	return max(toolkit.Scaled(statusH), toolkit.CurrentFont().Height()+toolkit.Scaled(8))
}

// How long drawing may take. Measured in render.Options' own doc comment: of
// 59 432 corpus pages, 1 131 were still being drawn after twenty seconds and
// one took two hundred and seventy-three. In a browser tab that is not a
// slow page, it is a hung one.
const (
	pageBudget  = 4 * time.Second
	thumbBudget = 400 * time.Millisecond
	// frameBudget bounds how long one repaint spends making thumbnails, so a
	// hundred-page document does not freeze the first frame.
	frameBudget = 120 * time.Millisecond
)

// uiFontPx is the logical size the editor sets its text at; UseUIFont
// multiplies it by the display's pixel ratio.
const uiFontPx = 14

// UseUIFont installs the UI typeface at the given scale.
//
// ⛔ THE FACE IS PART OF THE SCALE. Installing it once at 14 and then doubling
// every metric leaves the type the only thing on the surface that did not
// grow: labels that fitted their controls at 1x rattle around inside them at
// 2x, and a ribbon measured from its font lays out for a face it is not
// drawing with.
//
// A parse failure leaves whatever face is installed rather than refusing to
// start: an editor that opens with plain text beats one that does not open.
func UseUIFont(scale float64) {
	if !(scale > 0) {
		scale = 1
	}
	if f, err := toolkit.NewTrueTypeFont(ibmplexsans.TTF, int(float64(uiFontPx)*scale+0.5)); err == nil {
		toolkit.SetFont(f)
	}
}

// Scene is the whole editor: the widget tree, the document, and the bridge
// out to the browser for opening and saving.
type Scene struct {
	w, h int

	doc  *Doc
	acts map[string]*toolkit.Action

	ribbon *toolkit.Ribbon
	strip  *toolkit.PageStrip
	theme  *toolkit.Theme

	page   *toolkit.Image
	status string

	// rendered is the reader the page and the thumbnails are drawn from. It
	// is reparsed once per edit rather than once per draw.
	rendered *reader.Document

	// want is the queue of thumbnails the strip has asked for and not yet
	// been given, and deadline is how long this frame may spend making them.
	want     map[int]bool
	deadline time.Time

	// The open dialog, if any, and the command waiting on it.
	dialog          *toolkit.Dialog
	entry           *toolkit.Entry
	pending         Command
	confirm, cancel func()

	// OpenFile and SaveFile are the browser bridges, nil in a test.
	OpenFile func()
	SaveFile func(name string, data []byte)
}

// SetDark switches the editor between the light and dark themes.
//
// ⛔ THE CANVAS CANNOT INHERIT THE PAGE'S COLOURS. Everything around it is
// styled by landingkit's CSS tokens, which follow the reader's choice and the
// system setting; inside the canvas there is no CSS at all, so a light ribbon
// sits in a dark page looking like a bug until somebody is told it is not.
// The host reads the page's state and says so here.
func (s *Scene) SetDark(dark bool) {
	if dark {
		s.theme = toolkit.DefaultDark()
	} else {
		s.theme = toolkit.DefaultLight()
	}
}

// NewScene builds the editor with nothing open.
func NewScene(w, h int) *Scene {
	s := &Scene{w: w, h: h, want: map[int]bool{}, theme: toolkit.DefaultLight()}
	s.acts = actions(s)
	s.ribbon = buildRibbon(s.acts)
	s.strip = &toolkit.PageStrip{
		ThumbSize:   thumbW,
		PageW:       210,
		PageH:       297,
		MaxResident: 48,
		// ⛔ ANSWERED NOW WHERE THERE IS TIME, QUEUED WHERE THERE IS NOT.
		// PageStrip's own contract is that the host answers "now or later",
		// and relying on `later` alone cost a build: the ticker had not run a
		// frame before the first screenshot, so every card was still a
		// numbered placeholder — and a reader who glances once and leaves
		// sees exactly that screenshot.
		NeedImage: func(i int) {
			if s.thumb(i) {
				return
			}
			s.want[i] = true
		},
	}
	// Clicking a thumbnail turns to that page. The strip owns the selection
	// as an observable, so this is the only place the two are tied together.
	s.strip.Current().Subscribe(func(i int) {
		if s.doc != nil && i+1 != s.doc.Page() {
			s.doc.Show(i + 1)
			s.drawPage()
		}
	})
	s.status = "Open a PDF to begin — it is read in this tab and never uploaded."
	setEnabled(s.acts, nil)
	return s
}

// — Host, so the ribbon can reach the document and say things —

func (s *Scene) Doc() *Doc { return s.doc }

func (s *Scene) Say(msg string) { s.status = msg }

func (s *Scene) Open() {
	if s.OpenFile != nil {
		s.OpenFile()
	}
}

func (s *Scene) Save() {
	if s.SaveFile == nil || s.doc == nil {
		return
	}
	name := s.doc.Name
	if name == "" {
		name = "document.pdf"
	}
	s.SaveFile(name, s.doc.Bytes())
}

// Changed is called after every edit: the document's bytes are new, so the
// reader, the page and every thumbnail are stale.
func (s *Scene) Changed() {
	s.reparse()
	s.strip.SetCount(s.doc.PageCount())
	// ⛔ EVERY thumbnail, not the current one. A rotation changes one page, a
	// reverse changes all of them, and the strip has no way to know which —
	// so the honest answer is that they are all now unknown.
	s.strip.InvalidateAll()
	s.want = map[int]bool{}
	s.drawPage()
	setEnabled(s.acts, s.doc)
}

// Load takes a file the browser handed in.
func (s *Scene) Load(b []byte, name string) {
	d, err := Open(b, name)
	if err != nil {
		s.Say(err.Error())
		return
	}
	s.doc = d
	msg := fmt.Sprintf("%s — %d page(s)", name, d.PageCount())
	if d.Repaired {
		// ⛔ Said rather than swallowed: ops mends a damaged file instead of
		// refusing it, and a reader not told will wonder why this shows
		// something another viewer does not.
		msg += " — the file was damaged and has been repaired"
	}
	s.Say(msg)
	s.strip.Current().Set(0)
	s.Changed()
}

// reparse rebuilds the reader the pictures come from.
func (s *Scene) reparse() {
	s.rendered = nil
	if s.doc == nil {
		return
	}
	if d, err := reader.Open(s.doc.Bytes()); err == nil {
		s.rendered = d
	}
}

// drawPage renders the page being shown into the centre pane.
func (s *Scene) drawPage() {
	if s.rendered == nil || s.doc == nil || s.doc.PageCount() == 0 {
		s.page = nil
		return
	}
	// The picture is made at the size of the pane it goes in, so the fit is a
	// real fit and not an upscale of something small.
	avail := s.w - railW - 24
	if avail < 64 {
		avail = 64
	}
	img, err := render.Page(s.rendered, s.doc.Page(), render.Options{
		Scale:       scaleFor(avail),
		MaxDuration: pageBudget,
	})
	if err != nil {
		s.page = nil
		s.Say(fmt.Sprintf("page %d could not be drawn: %v", s.doc.Page(), err))
		return
	}
	s.page = toolkit.NewImageFit(img.Pix, img.W, img.H)
}

// fillBG paints every pixel of the buffer the theme's background colour.
//
// It writes the four bytes directly rather than going through the painter: it
// is the one operation that touches the whole surface every frame, and a
// window is several million pixels.
func fillBG(buf []byte, c toolkit.RGBA) {
	if len(buf) < 4 {
		return
	}
	buf[0], buf[1], buf[2], buf[3] = c.R, c.G, c.B, 0xff
	// Doubling copy: each pass fills twice what is already filled, so the
	// whole buffer is written in log2(n) copies rather than n stores.
	for n := 4; n < len(buf); n *= 2 {
		copy(buf[n:], buf[:n])
	}
}

// scaleFor turns a pane width in pixels into render's device-pixels-per-point.
// A4 is 595 points across, which is what most of a corpus is.
func scaleFor(px int) float64 {
	s := float64(px) / 595
	switch {
	case s < 0.3:
		return 0.3
	case s > 3:
		return 3
	}
	return s
}

// Tick makes the thumbnails the strip has asked for, a few per frame.
//
// ⛔ BOUNDED BY TIME, NOT BY COUNT. Pages differ by orders of magnitude in
// what they cost to draw — the same corpus that renders most pages in
// milliseconds has one that takes minutes — so "three per frame" is a budget
// that means nothing. A deadline means the same thing on every document.
// ⛔ IT RETURNS NOTHING, AND THAT MATTERS. webcanvas.Ticker is `Tick()`; a
// `Tick() bool` does not satisfy it, the interface is not implemented, no
// timer is installed and this is never called — with no error anywhere,
// because nothing asked for the interface. The thumbnails stayed as numbered
// placeholders and everything compiled. cmd/playground-wasm now asserts the
// three webcanvas interfaces at compile time so the next such slip is a build
// failure rather than a missing feature.
func (s *Scene) Tick() {
	if s.rendered == nil || len(s.want) == 0 {
		return
	}
	s.deadline = time.Now().Add(frameBudget)
	for i := range s.want {
		if !s.thumb(i) {
			break // out of time for this frame; the rest wait for the next
		}
		delete(s.want, i)
	}
}

// thumb draws one thumbnail and hands it to the strip. It reports whether it
// did: false means there was no time left in this frame, or the page refused
// to draw within its own budget.
//
// ⛔ The card keeps its numbered placeholder when this says no. An empty box
// would read as a blank page, which is a thing PDFs genuinely contain, and a
// reader cannot tell the two apart.
func (s *Scene) thumb(i int) bool {
	if s.rendered == nil || i < 0 || i >= s.strip.Count() {
		return false
	}
	if time.Now().After(s.deadline) {
		return false
	}
	img, err := render.Page(s.rendered, i+1, render.Options{
		// ⛔ At the DISPLAY's resolution. The card is physically wider on a
		// Retina screen, so a bitmap made at the logical DPI is one the strip
		// scales up with nearest-neighbour — a blurrier card than the one a
		// reader would have had at 1x.
		DPI:         thumbDPI * toolkit.MetricScale(),
		MaxDuration: thumbBudget,
	})
	if err != nil {
		// Drawn once, refused once: do not ask again this frame, and do not
		// spend the rest of the budget on a page that will refuse again.
		return true
	}
	s.strip.SetImage(i, img.Pix, img.W, img.H)
	return true
}

// — webcanvas.App —

func (s *Scene) Size() (int, int) { return s.w, s.h }

// SetPixelRatio tells the editor how many device pixels the display puts in a
// CSS pixel, and is webcanvas's [PixelRatioAware] contract.
//
// ⛔ IMPLEMENTING IT IS THE WHOLE FIX. A scene that omits this interface is
// handed CSS pixels and its canvas is sized in CSS pixels — webcanvas says so
// outright — and this editor omitted it while its wasm shell called
// SetMetricScale(devicePixelRatio) behind webcanvas's back. Every widget was
// therefore drawn twice as large onto a surface that had not grown, so on a
// Retina display the ribbon covered the page and there was nothing to see.
//
// Saying so here instead gets the rest for free: webcanvas sizes the backing
// store to the CSS box times the ratio, hands Resize DEVICE pixels, and calls
// this again when the ratio changes — a browser zoom, or a window dragged to a
// display of another density. The shell's own call could not do that: it read
// the ratio once, at startup.
func (s *Scene) SetPixelRatio(ratio float64) {
	if !(ratio > 0) { // also catches NaN
		ratio = 1
	}
	if ratio == toolkit.MetricScale() {
		return
	}
	toolkit.SetMetricScale(ratio)
	// The face is installed at the scale as well, or the type stays the size it
	// was while everything around it grows.
	UseUIFont(ratio)
	// ⛔ ThumbSize is NOT touched: PageStrip says it is a LOGICAL width and
	// scales it itself, so setting it to a scaled number would apply the ratio
	// twice and give a card four times the area at ratio 2.
	//
	// What does have to follow is the resolution the pictures are drawn at. A
	// card that is physically twice as wide showing a bitmap made for a 1x
	// card is the one case where a bigger card really is a blurrier one, so
	// every page already drawn is marked stale and asked for again — the old
	// bitmap stays on screen until its replacement arrives.
	if s.strip != nil {
		s.strip.InvalidateAll()
		s.want = map[int]bool{}
	}
}

func (s *Scene) Resize(w, h int) (int, int) {
	// ⛔ The floor is in DEVICE pixels, so it follows the ratio too: a 320 that
	// did not would refuse a window that is 320 CSS pixels wide on a Retina
	// display and lay the editor out at twice the size of the canvas.
	if min := toolkit.Scaled(320); w < min {
		w = min
	}
	if min := toolkit.Scaled(240); h < min {
		h = min
	}
	if w == s.w && h == s.h {
		return s.w, s.h
	}
	s.w, s.h = w, h
	// The page was drawn to fit the old pane, so it is now the wrong size.
	s.drawPage()
	return s.w, s.h
}

// Bounds of the three regions. They are computed rather than stored so that a
// resize cannot leave one of them describing the old window.
func (s *Scene) ribbonRect() painter.Rect {
	return painter.Rect{X: 0, Y: 0, W: s.w, H: s.ribbonHeight()}
}

func (s *Scene) stripRect() painter.Rect {
	top := s.ribbonHeight()
	return painter.Rect{X: 0, Y: top, W: railWidth(), H: s.h - top - statusHeight()}
}

func (s *Scene) pageRect() painter.Rect {
	top, g := s.ribbonHeight(), toolkit.Scaled(gutter)
	return painter.Rect{X: railWidth() + g, Y: top + g,
		W: s.w - railWidth() - 2*g, H: s.h - top - statusHeight() - 2*g}
}

// ribbonHeight is ASKED OF THE RIBBON rather than fixed at 128. The ribbon
// works its height out from its own metrics — the tab strip, the body, the
// group labels — so it is the only answer that cannot disagree with what is
// painted, and it follows the metric scale for nothing.
func (s *Scene) ribbonHeight() int { return s.ribbon.Height() }

func (s *Scene) Draw(buf []byte) {
	// ⛔ THE BACKGROUND IS PAINTED FIRST, AND THAT IS NOT DECORATION. The host
	// hands over a buffer it does not clear, and a fresh one is all zeroes —
	// which is opaque BLACK, not blank. Without this the window is black
	// wherever no widget happens to land, and every "is anything drawn?"
	// check answers yes for the wrong reason. This was found by removing the
	// ribbon's Draw and watching the test that counts dark pixels still pass.
	fillBG(buf, s.theme.Background)

	// The strip asks for thumbnails from inside its own Draw, and this is the
	// budget those requests are answered within.
	s.deadline = time.Now().Add(frameBudget)

	p := painter.NewPixelPainter(buf, s.w, s.h)

	s.ribbon.SetBounds(s.ribbonRect())
	s.ribbon.Draw(p, s.theme)

	s.strip.SetBounds(s.stripRect())
	s.strip.Draw(p, s.theme)

	if s.page != nil {
		s.page.SetBounds(s.pageRect())
		s.page.Draw(p, s.theme)
	}

	lbl := toolkit.NewLabel(s.status)
	ins, sh := toolkit.Scaled(pad), statusHeight()
	lbl.SetBounds(painter.Rect{X: ins, Y: s.h - sh, W: s.w - 2*ins, H: sh})
	lbl.Draw(p, s.theme)

	// Last, and over everything: a modal that something else painted on top
	// of would be a question the reader cannot read.
	s.drawDialog(p)
}

func (s *Scene) Click(x, y int) bool   { return s.event(toolkit.EventClick, x, y) }
func (s *Scene) Move(x, y int) bool    { return s.event(toolkit.EventMouseMove, x, y) }
func (s *Scene) Release(x, y int) bool { return s.event(toolkit.EventMouseUp, x, y) }
func (s *Scene) Context(x, y int) bool { return s.event(toolkit.EventSecondaryClick, x, y) }

// Char is text being typed, which only a dialog's field has any use for.
func (s *Scene) Char(str string) bool {
	if s.dialog == nil {
		return false
	}
	s.dialog.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: str})
	return true
}

// KeyDown turns the keys a reader expects in a PDF viewer into commands.
func (s *Scene) KeyDown(k string) bool {
	if s.dialog != nil {
		return s.dialogKey(k)
	}
	if s.doc == nil {
		return false
	}
	switch k {
	case "ArrowRight", "PageDown", " ":
		s.show(s.doc.Page() + 1)
	case "ArrowLeft", "PageUp":
		s.show(s.doc.Page() - 1)
	case "Home":
		s.show(1)
	case "End":
		s.show(s.doc.PageCount())
	default:
		return false
	}
	return true
}

func (s *Scene) show(n int) {
	s.doc.Show(n)
	s.strip.Current().Set(s.doc.Page() - 1)
	s.strip.ScrollTo(s.doc.Page() - 1)
	s.drawPage()
}

// event hands a pointer event to the ribbon and to the strip.
//
// Both of them, rather than whichever contains the point: a widget that is not
// under the pointer ignores what it is given, and the ribbon in particular has
// popups that extend well past the bounds it was laid out in.
func (s *Scene) event(kind toolkit.EventKind, x, y int) bool {
	ev := toolkit.Event{Kind: kind, X: x, Y: y}

	// ⛔ A MODAL TAKES EVERYTHING. Letting the ribbon see a click while a
	// dialog is open is what makes a question answerable by pressing
	// something else, and leaves the dialog waiting on an answer that has
	// already been given somewhere behind it.
	if s.dialog != nil {
		s.dialog.SetBounds(s.dialogRect())
		s.dialog.OnEvent(ev)
		return true
	}

	s.ribbon.SetBounds(s.ribbonRect())
	s.ribbon.OnEvent(ev)
	s.strip.SetBounds(s.stripRect())
	s.strip.OnEvent(ev)
	return true
}
