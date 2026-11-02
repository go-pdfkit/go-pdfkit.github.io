// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Command pagecheck serves the built site, drives a real browser at the
// playground, and READS THE PIXELS BACK.
//
// ⛔ THE DOM CANNOT ANSWER THIS. The whole interface is painted by the wasm
// module into one <canvas>, so the document object model holds a canvas
// element and nothing else whether the editor started or never ran. Counting
// elements would pass for a blank rectangle. The only honest question is what
// is on the screen, so this takes a screenshot and looks at it.
//
// It is the automation of a check first done by hand, and it keeps what that
// check caught: the editor must DRAW, not merely load.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	dir := flag.String("dir", "../public", "the built site")
	chrome := flag.String("chrome", "", "the browser binary")
	shot := flag.String("shot", "", "keep the screenshot here (optional)")
	budget := flag.Duration("budget", 90*time.Second, "how long the page is given")
	flag.Parse()

	if err := check(*dir, strings.TrimSpace(*chrome), *shot, *budget); err != nil {
		fmt.Fprintf(os.Stderr, "pagecheck: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("the editor started, drew its ribbon and drew a page")
}

// The window the browser is given. Large enough that the ribbon, the strip
// and the page all have somewhere to be.
const (
	winW = 1280
	winH = 900
)

func check(dir, chrome, shot string, budget time.Duration) error {
	if chrome == "" {
		return fmt.Errorf("no browser given (-chrome)")
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("the built site is not at %s: %w", dir, err)
	}
	// ⛔ application/wasm is set here: Go's mime table does not always know
	// the extension, and a module served as octet-stream fails to instantiate
	// with a message about MIME types that reads like a build problem.
	if err := mime.AddExtensionType(".wasm", "application/wasm"); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	go srv.Serve(ln)
	defer srv.Close()

	tmp, err := os.MkdirTemp("", "pagecheck")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "shot.png")

	url := fmt.Sprintf("http://%s/playground/", ln.Addr().String())
	cmd := exec.Command(chrome,
		"--headless", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", winW, winH),
		// Virtual time lets the module download, start and paint without this
		// program guessing a sleep.
		fmt.Sprintf("--virtual-time-budget=%d", budget.Milliseconds()),
		"--screenshot="+out, url)
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	// ⛔ THE SCREENSHOT IS THE EVIDENCE, NOT THE EXIT STATUS. Headless Chrome
	// writes the image and then sometimes fails to tear itself down
	// ("Teardown watchdog expired", exit 2) for reasons that have nothing to
	// do with the page — and treating that as a failure would make this check
	// red on a page that is perfectly fine, which is the fastest way to teach
	// everyone to ignore it. If there is no image, the exit status is the
	// only thing to report and it IS the failure.
	f, err := os.Open(out)
	if err != nil {
		if runErr != nil {
			return fmt.Errorf("the browser failed and wrote no screenshot: %w", runErr)
		}
		return fmt.Errorf("the browser wrote no screenshot: %w", err)
	}
	if runErr != nil {
		fmt.Printf("note: the browser exited %v after writing the screenshot; judging the image\n", runErr)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("the screenshot will not decode: %w", err)
	}
	if shot != "" {
		if b, err := os.ReadFile(out); err == nil {
			_ = os.WriteFile(shot, b, 0o644)
		}
	}
	return judge(img)
}

// judge decides whether what was drawn is an editor.
func judge(img image.Image) error {
	b := img.Bounds()
	if b.Dx() < 400 || b.Dy() < 300 {
		return fmt.Errorf("the screenshot is %dx%d, which is not the window asked for",
			b.Dx(), b.Dy())
	}

	// ⛔ THE BOOT OVERLAY IS STILL THERE IF THE MODULE NEVER STARTED, and it
	// is a wall of background colour with a line of text. So the check is not
	// "is there ink anywhere" — it is "is there ink in the band where the
	// ribbon lives, and in the band where a page lives".
	//
	// The stage begins below the site header and the heading; measuring from
	// the bottom of the image avoids depending on their exact height.
	stageTop := b.Min.Y + b.Dy()/4
	ribbonBand := image.Rect(b.Min.X, stageTop, b.Max.X, stageTop+b.Dy()/10)
	pageBand := image.Rect(b.Min.X+b.Dx()/3, stageTop+b.Dy()/5, b.Max.X, b.Max.Y-10)

	// The thumbnail strip runs down the left of the stage.
	stripBand := image.Rect(b.Min.X+b.Dx()/14, stageTop+b.Dy()/5, b.Min.X+b.Dx()/5, b.Max.Y-10)

	ribbonInk := inkFraction(img, ribbonBand)
	pageInk := inkFraction(img, pageBand)
	stripPaper := paperFraction(img, stripBand)

	// ⛔ THE STRIP MUST HOLD PAPER, NOT PLACEHOLDERS. PageStrip draws a
	// numbered card for every page it has no bitmap for, so a strip of
	// placeholders looks populated and scrolls correctly while showing
	// nothing of the document. This check passed for a whole build in exactly
	// that state, because "is the strip drawn" and "are the pages drawn" are
	// different questions. A rendered page is white; a placeholder card is
	// the theme's panel colour, which is never paper-white in either theme.
	if stripPaper < 0.10 {
		return fmt.Errorf("the thumbnail strip is %.1f%% paper — the cards are placeholders, "+
			"not rendered pages", 100*stripPaper)
	}

	// A ribbon is mostly labels on a bar: sparse ink, but not none.
	if ribbonInk < 0.004 {
		return fmt.Errorf("the ribbon band is %.3f%% ink — the editor did not draw its commands",
			100*ribbonInk)
	}
	// A rendered page is a white sheet with a border and some marks on it, on
	// a darker stage: there is always an edge.
	if pageInk < 0.001 {
		return fmt.Errorf("the page area is %.3f%% ink — nothing was rendered", 100*pageInk)
	}
	// ⛔ The control on the control. A screenshot that came back entirely one
	// colour passes neither test above by luck; say which it was, because
	// "0.000%% ink" from a solid-black image reads like a drawing defect
	// rather than like a browser that rendered nothing at all.
	if uniform(img, b) {
		return fmt.Errorf("the whole screenshot is one colour — the page never painted")
	}
	fmt.Printf("ribbon band %.2f%% ink, page area %.2f%% ink, strip %.1f%% paper\n",
		100*ribbonInk, 100*pageInk, 100*stripPaper)
	return nil
}

// paperFraction is the share of pixels in r that are paper-white — which a
// rendered page is and a placeholder card is not, in either theme.
func paperFraction(img image.Image, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	n, white := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			n++
			if cr>>8 > 240 && cg>>8 > 240 && cb>>8 > 240 {
				white++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(white) / float64(n)
}

// inkFraction is the share of pixels in r that are appreciably darker than
// paper.
func inkFraction(img image.Image, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	n, dark := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			n++
			if int(cr>>8)+int(cg>>8)+int(cb>>8) < 3*190 {
				dark++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(dark) / float64(n)
}

// uniform reports whether every pixel is the same colour.
func uniform(img image.Image, r image.Rectangle) bool {
	first := true
	var fr, fg, fb uint32
	for y := r.Min.Y; y < r.Max.Y; y += 4 {
		for x := r.Min.X; x < r.Max.X; x += 4 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if first {
				fr, fg, fb, first = cr, cg, cb, false
				continue
			}
			if cr != fr || cg != fg || cb != fb {
				return false
			}
		}
	}
	return true
}
