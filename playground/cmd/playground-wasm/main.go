// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build js && wasm

// Command playground-wasm is the browser half of the playground: it runs a
// recipe over a PDF and renders the result's pages, entirely inside the tab.
//
// ⛔ NOTHING IS UPLOADED, AND THAT IS THE FEATURE. There is no endpoint to
// send a file to, which is not a promise about our conduct but a property of
// the build: the whole tool is this wasm module and a static page.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"syscall/js"

	playground "github.com/go-pdfkit/go-pdfkit.github.io/playground"
	"github.com/go-pdfkit/reader"
	"github.com/go-pdfkit/render"
)

// previewDPI is what the pages are rasterised at for the preview. It is a
// preview and not the artefact: the PDF the reader downloads is the vector
// document, and this is only what is shown.
const previewDPI = 72

// maxPreviewPages bounds the work one run can ask for. A recipe may turn eight
// pages into sixty-four (divide 4 2 twice over), and rendering all of them
// freezes the tab for no benefit — nobody reads page sixty of a preview.
const maxPreviewPages = 12

// out is the shape handed back to JavaScript. Everything the page needs is in
// one object, so the boundary is crossed once per run.
type out struct {
	Pages    int      `json:"pages"`
	Rendered int      `json:"rendered"`
	PDF      string   `json:"pdf"`      // base64, empty on failure
	Previews []string `json:"previews"` // base64 PNG, one per rendered page
	Log      []string `json:"log"`
	Err      string   `json:"err"`
}

func fail(msg string) string {
	b, _ := json.Marshal(out{Err: msg})
	return string(b)
}

// run is the one entry point: it takes the recipe and the PDF, and returns
// JSON. It is deliberately a single call rather than a handle-and-methods API
// — a playground run has no state worth keeping between presses.
func run(this js.Value, args []js.Value) any {
	if len(args) != 2 {
		return fail("run takes the recipe and the PDF")
	}
	src := args[0].String()

	in := make([]byte, args[1].Get("length").Int())
	js.CopyBytesToGo(in, args[1])

	r := playground.Run(src, in)
	if r.Err != "" {
		// ⛔ The log comes back even on failure: it is how far it got, which
		// is the first thing anybody wants to know.
		b, _ := json.Marshal(out{Err: r.Err, Log: r.Log})
		return string(b)
	}

	o := out{
		Pages: r.Pages,
		Log:   r.Log,
		PDF:   base64.StdEncoding.EncodeToString(r.PDF),
	}

	d, err := reader.Open(r.PDF)
	if err != nil {
		// The document was written but cannot be read back. Say so rather
		// than showing an empty preview beside a working download button.
		o.Err = "the result could not be re-opened for preview: " + err.Error()
		b, _ := json.Marshal(o)
		return string(b)
	}
	n := d.PageCount()
	if n > maxPreviewPages {
		n = maxPreviewPages
	}
	for p := 1; p <= n; p++ {
		img, err := render.Page(d, p, render.Options{DPI: previewDPI})
		if err != nil {
			o.Log = append(o.Log, fmt.Sprintf("page %d could not be drawn: %v", p, err))
			continue
		}
		// ⛔ ToNRGBA, not the raster.Image itself. go-gfx's Image deliberately
		// is NOT an image.Image — its At returns a color.RGBA rather than a
		// color.Color, so the hot path does no interface boxing. Converting
		// here is the price of handing it to the standard encoder, and it is
		// paid once per previewed page rather than once per pixel.
		var buf bytes.Buffer
		if err := png.Encode(&buf, img.ToNRGBA()); err != nil {
			o.Log = append(o.Log, fmt.Sprintf("page %d could not be encoded: %v", p, err))
			continue
		}
		o.Previews = append(o.Previews, base64.StdEncoding.EncodeToString(buf.Bytes()))
	}
	o.Rendered = len(o.Previews)

	b, _ := json.Marshal(o)
	return string(b)
}

// sample hands back the document the playground opens with, so the page works
// before anybody has a file to try.
func sample(this js.Value, args []js.Value) any {
	b, err := playground.ExamplePDF()
	if err != nil {
		return fail("the example document could not be built: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(b)
}

// verbs hands back the language, generated from the table rather than typed
// into the page, so the help cannot drift from what runs.
func verbs(this js.Value, args []js.Value) any {
	b, _ := json.Marshal(playground.Verbs())
	return string(b)
}

func main() {
	js.Global().Set("pdfkitRun", js.FuncOf(run))
	js.Global().Set("pdfkitSample", js.FuncOf(sample))
	js.Global().Set("pdfkitVerbs", js.FuncOf(verbs))
	js.Global().Set("pdfkitExampleRecipe", js.ValueOf(playground.ExampleRecipe))
	// ⛔ Set LAST, and only once the functions above exist. The page waits on
	// this flag; a page that started calling pdfkitRun because the module had
	// loaded would find the global undefined.
	js.Global().Set("pdfkitReady", js.ValueOf(true))
	select {} // a wasm main that returns takes its exports with it
}
