// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build js && wasm

// Command playground-wasm runs the PDF editor in a browser tab.
//
// ⛔ NOTHING IS UPLOADED, AND THAT IS A PROPERTY OF THE BUILD rather than a
// promise: the whole tool is this module and a static page, and there is no
// endpoint for a file to go to. Opening one reads it through the browser's
// own file input; saving one makes a blob and clicks a link at it.
package main

import (
	"syscall/js"

	playground "github.com/go-pdfkit/go-pdfkit.github.io/playground"
	"github.com/go-widgets/webcanvas"
)

// canvasID is the element the scene is painted into.
const canvasID = "pdf-canvas"

// ⛔ THE INTERFACES ARE ASSERTED HERE, AT COMPILE TIME. webcanvas takes an
// `any` and asks it for these at run time, so a method with the wrong
// signature is not an error — it is a feature that silently never happens.
// A `Tick() bool` instead of `Tick()` cost exactly that: the thumbnails never
// rendered, nothing failed, and everything compiled.
var (
	_ webcanvas.App             = (*playground.Scene)(nil)
	_ webcanvas.Ticker          = (*playground.Scene)(nil)
	_ webcanvas.Resizer         = (*playground.Scene)(nil)
	_ webcanvas.PixelRatioAware = (*playground.Scene)(nil)
)

func main() {
	// The UI typeface. Without it every label measures as nothing and the
	// ribbon lays out as a row of empty boxes — which looks like a layout
	// defect and is a missing font.
	//
	// A parse failure leaves the toolkit's bitmap face in place rather than
	// refusing to start: an editor that opens with plain text beats one that
	// does not open.
	playground.UseUIFont(1)
	// ⛔ THE RATIO IS NOT READ HERE ANY MORE, and that is the fix. This used to
	// read window.devicePixelRatio and call toolkit.SetMetricScale with it —
	// behind webcanvas's back, which went on sizing the canvas in CSS pixels
	// because the scene did not claim to want device ones. Every widget was
	// drawn twice as large onto a surface that had not grown, so on a Retina
	// display the ribbon covered the page and there was nothing to see.
	//
	// The scene implements webcanvas.PixelRatioAware instead, so webcanvas
	// tells it the ratio, sizes the backing store, and tells it AGAIN when a
	// zoom or another display changes it. A number read once at startup could
	// never have followed that.
	s := playground.NewScene(1100, 760)
	s.OpenFile = openFile
	s.SaveFile = saveFile
	followPageTheme(s)

	// ⛔ THE EXAMPLE IS LOADED HERE, IN GO, AND NOT BY THE PAGE. It was the
	// page's job for one build, and the editor came up empty with every
	// command greyed out: a call that crosses into JavaScript and back is one
	// more thing between a reader and the first thing they see. Opening on a
	// document is not a convenience — an editor that opens empty asks the
	// reader to find a file before it has shown them anything at all.
	if b, err := playground.ExamplePDF(); err == nil {
		s.Load(b, "example.pdf")
	}

	// ⛔ Set before Run, which does not return. A page that began calling in
	// before this would find the globals undefined.
	js.Global().Set("pdfkitLoad", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 {
			return nil
		}
		b := make([]byte, args[0].Get("length").Int())
		js.CopyBytesToGo(b, args[0])
		s.Load(b, args[1].String())
		return nil
	}))
	// Kept so the page can put the example back after somebody has opened
	// their own file, but the editor no longer depends on it to start.
	js.Global().Set("pdfkitSample", js.FuncOf(func(js.Value, []js.Value) any {
		if b, err := playground.ExamplePDF(); err == nil {
			s.Load(b, "example.pdf")
		}
		return nil
	}))
	js.Global().Set("pdfkitReady", js.ValueOf(true))

	webcanvas.Run(canvasID, s)
}

// followPageTheme keeps the canvas in step with the page around it, and goes on
// doing so.
//
// ⛔ A VALUE READ ONCE AT START-UP CANNOT FOLLOW A CHANGE. This is the same
// mistake the pixel ratio made seven lines above, left standing for the theme:
// SetDark was called once in main and never again, so a reader who pressed the
// theme toggle, or whose machine turned dark at sunset, kept a light ribbon in a
// dark page until they reloaded.
//
// There are TWO sources and the kit's default state only reacts to the second:
//
//   - data-theme on <html>, which is what landingkit's three-state toggle
//     writes when the reader chooses light or dark;
//   - prefers-color-scheme, which is what "system" — the default — resolves
//     through, and which changes with no gesture on the page at all.
//
// Neither is read directly. pageIsDark reads the COMPUTED background, so
// whichever of the two moved, and whatever the sheet does with it, the answer
// is the colour actually painted. Both listeners simply ask again.
//
// go-slides does this from the page instead, pushing the computed palette into
// the module (goSlidesSetPalette). In Go it travels with the binary, so a page
// that embeds this playground gets it without copying any script.
//
// The callbacks are never released, which is correct: they live as long as the
// page does, and a released js.Func called from JavaScript panics.
func followPageTheme(s *playground.Scene) {
	sync := func() { s.SetDark(pageIsDark()) }
	sync()
	again := js.FuncOf(func(js.Value, []js.Value) any { sync(); return nil })

	doc := js.Global().Get("document")
	if obs := js.Global().Get("MutationObserver"); obs.Truthy() && doc.Truthy() {
		if root := doc.Get("documentElement"); root.Truthy() {
			obs.New(again).Call("observe", root, map[string]any{
				"attributes":      true,
				"attributeFilter": []any{"data-theme"},
			})
		}
	}
	if js.Global().Get("matchMedia").Truthy() {
		q := js.Global().Call("matchMedia", "(prefers-color-scheme: dark)")
		// addEventListener on a MediaQueryList is the modern spelling; Safari
		// before 14 had only addListener, and a browser with neither simply does
		// not follow the system — it still starts on the right colour.
		switch {
		case q.Get("addEventListener").Truthy():
			q.Call("addEventListener", "change", again)
		case q.Get("addListener").Truthy():
			q.Call("addListener", again)
		}
	}
}

// pageIsDark asks the page which way round it is.
//
// It reads the COMPUTED background of the body rather than a class name or
// the media query alone: landingkit offers light, dark and "system", and only
// the computed colour knows which of the three the reader ended up with.
func pageIsDark() bool {
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		return false
	}
	body := doc.Get("body")
	if body.IsUndefined() || body.IsNull() {
		return false
	}
	style := js.Global().Call("getComputedStyle", body)
	if style.IsUndefined() {
		return false
	}
	return playground.IsDarkColour(style.Get("backgroundColor").String())
}

// openFile asks the page's hidden file input for a document.
func openFile() {
	el := js.Global().Get("document").Call("getElementById", "pdf-file")
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Call("click")
}

// saveFile hands the document to the browser as a download.
//
// The blob URL is revoked once the click has been dispatched: an editor is
// saved from many times in a sitting, and each one would otherwise hold its
// document in memory for the life of the tab.
func saveFile(name string, data []byte) {
	u8 := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(u8, data)
	parts := js.Global().Get("Array").New()
	parts.Call("push", u8)
	blob := js.Global().Get("Blob").New(parts,
		map[string]any{"type": "application/pdf"})
	url := js.Global().Get("URL").Call("createObjectURL", blob)

	a := js.Global().Get("document").Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", name)
	a.Call("click")
	js.Global().Get("URL").Call("revokeObjectURL", url)
}
