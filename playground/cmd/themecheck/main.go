// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Command themecheck proves, in a real browser, that the canvas follows the
// page's theme — both when the reader chooses one and when the machine changes
// its mind underneath them.
//
// ⛔ A VALUE READ ONCE AT START-UP CANNOT FOLLOW A CHANGE, and a check that
// loads a page once cannot see whether it did. cmd/pagecheck asks Chrome for a
// single screenshot and judges it, which is the right shape for "did the editor
// draw"; it is the wrong shape for this, because the defect this guards is
// invisible until something changes AFTER the page has settled.
//
// So the three states are driven, in order, on one loaded page:
//
//	light          the system setting, emulated      the canvas is light
//	dark           the system setting, changed       the canvas turns dark
//	data-theme     the reader's toggle, written      the canvas turns light
//
// The second proves the prefers-color-scheme listener, the third the
// MutationObserver. Before the listeners existed the canvas held whatever it
// read in main() and both of the last two rows failed.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// The window the page is given. Wide enough for the ribbon to lay out as it
// does for a reader, which is what the bands below are fractions of.
const (
	winW = 1280
	winH = 900
)

func main() {
	dir := flag.String("dir", "../public", "the built site")
	chrome := flag.String("chrome", "", "the browser to drive")
	patience := flag.Duration("patience", 120*time.Second, "how long each state is given to settle")
	flag.Parse()
	if err := run(*dir, *chrome, *patience); err != nil {
		fmt.Fprintln(os.Stderr, "themecheck:", err)
		os.Exit(1)
	}
	fmt.Println("the canvas followed the system setting and the reader's toggle")
}

func run(dir, chrome string, patience time.Duration) error {
	if chrome == "" {
		return fmt.Errorf("no browser given (-chrome)")
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("the built site is not at %s: %w", dir, err)
	}
	// ⛔ As in pagecheck: Go's mime table does not always know .wasm, and a
	// module served as octet-stream fails to instantiate with a message about
	// MIME types that reads like a build problem.
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

	ctx, cancel := context.WithTimeout(context.Background(), patience*4)
	defer cancel()

	cmd := exec.Command(chrome,
		"--headless", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", winW, winH),
		"--remote-debugging-port=0",
		"about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", chrome, err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	url, err := devtoolsURL(stderr)
	if err != nil {
		return err
	}
	c, err := dial(ctx, url)
	if err != nil {
		return err
	}
	defer c.close()
	session, err := attach(ctx, c)
	if err != nil {
		return err
	}
	for _, m := range []string{"Page.enable", "Runtime.enable"} {
		if _, err := c.call(ctx, m, nil, session); err != nil {
			return err
		}
	}
	return drive(ctx, c, session, fmt.Sprintf("http://%s/playground/", ln.Addr().String()), patience)
}

// drive loads the page in light, then changes the world twice.
func drive(ctx context.Context, c *conn, session, page string, patience time.Duration) error {
	// ⛔ THE SETTING IS EMULATED BEFORE THE PAGE LOADS. Set afterwards, the
	// first reading would be whatever the runner's own desktop says, which is
	// light on a Linux runner and dark on this author's Mac — a check whose
	// starting point depends on whose machine it runs on has no starting point.
	if err := scheme(ctx, c, session, "light"); err != nil {
		return err
	}
	if _, err := c.call(ctx, "Page.navigate", map[string]any{"url": page}, session); err != nil {
		return err
	}
	if err := until(patience, "the editor to start", func() (bool, error) {
		v, err := eval(ctx, c, session, "String(!!window.pdfkitReady)")
		return v == "true", err
	}); err != nil {
		return err
	}
	// The module sets pdfkitReady before webcanvas.Run, so the first frame is
	// still to come. Wait for the canvas itself to be something other than one
	// colour — asked of the canvas, because an unpainted canvas sits inside a
	// page that is already fully drawn.
	if err := until(patience, "the editor to paint", func() (bool, error) {
		r, err := canvasRect(ctx, c, session)
		if err != nil {
			return false, err
		}
		b, err := shot(ctx, c, session)
		if err != nil {
			return false, err
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return false, err
		}
		return !flat(img, r.ribbon()), nil
	}); err != nil {
		return err
	}

	light, err := brightness(ctx, c, session)
	if err != nil {
		return err
	}

	// The machine turns dark underneath the reader. Nothing on the page is
	// touched: this is the listener the kit's DEFAULT state ("system") needs,
	// and the one nothing else can stand in for.
	if err := scheme(ctx, c, session, "dark"); err != nil {
		return err
	}
	dark, err := settle(ctx, c, session, patience, "the canvas to follow the system into dark",
		func(now float64) bool { return now < light-minSwing })
	if err != nil {
		return fmt.Errorf("%w (it was %.0f and stayed there; the system listener never fired)", err, light)
	}

	// The reader presses the toggle, which writes data-theme on <html>. The
	// system setting is still dark, so only the attribute can explain a change.
	if _, err := eval(ctx, c, session,
		`document.documentElement.setAttribute("data-theme","light"), "ok"`); err != nil {
		return err
	}
	back, err := settle(ctx, c, session, patience, "the canvas to follow the reader's toggle back to light",
		func(now float64) bool { return now > dark+minSwing })
	if err != nil {
		return fmt.Errorf("%w (it was %.0f and stayed there; the data-theme observer never fired)", err, dark)
	}

	fmt.Printf("ribbon brightness: light %.0f, system dark %.0f, toggled light %.0f (of 255)\n",
		light, dark, back)
	return nil
}

// minSwing is how far the ribbon's mean brightness must move for a change to
// be a change rather than a repaint. The two themes are far apart — a light
// ribbon is in the 230s and a dark one in the 20s — so this is loose on
// purpose: it is here to refuse NOISE, not to measure the palette.
const minSwing = 40

// settle waits for the ribbon's brightness to satisfy want, and returns it.
func settle(ctx context.Context, c *conn, session string, patience time.Duration, what string, want func(float64) bool) (float64, error) {
	var last float64
	err := until(patience, what, func() (bool, error) {
		v, err := brightness(ctx, c, session)
		if err != nil {
			return false, err
		}
		last = v
		return want(v), nil
	})
	return last, err
}

// brightness is the mean channel value of the canvas's ribbon band: the
// clearest thing that differs between the two themes, and one that needs no
// knowledge of which colours the toolkit chose.
func brightness(ctx context.Context, c *conn, session string) (float64, error) {
	r, err := canvasRect(ctx, c, session)
	if err != nil {
		return 0, err
	}
	b, err := shot(ctx, c, session)
	if err != nil {
		return 0, err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	band := r.ribbon()
	if band.Intersect(img.Bounds()).Empty() {
		return 0, fmt.Errorf("the canvas's ribbon band %v is outside the %v screenshot — "+
			"the editor is scrolled off the page this check photographs", band, img.Bounds())
	}
	return mean(img, band), nil
}

// ⛔ THE BAND IS ASKED OF THE CANVAS, NOT GUESSED AS A FRACTION OF THE IMAGE.
// The first version of this check took the ribbon to be a slice a quarter of
// the way down the screenshot, as pagecheck does — and a quarter of the way
// down is the PAGE, above the canvas, which follows the stylesheet whatever
// the module does. It read 248 → 22 → 248 and passed, and it passed just the
// same with the listeners taken back out. A check measuring the wrong
// rectangle is not a weak check, it is a green light wired to nothing.
//
// So the canvas says where it is, and the band is inside it.
type rect struct {
	X, Y, W, H, D float64
}

// canvasRect asks the page for the canvas's box and the device pixel ratio.
// The screenshot is in DEVICE pixels and getBoundingClientRect answers in CSS
// pixels, so the two only agree when the ratio is 1.
func canvasRect(ctx context.Context, c *conn, session string) (rect, error) {
	v, err := eval(ctx, c, session, `(() => {
		const e = document.getElementById("pdf-canvas");
		if (!e) return "";
		const r = e.getBoundingClientRect();
		return JSON.stringify({X:r.x, Y:r.y, W:r.width, H:r.height, D:window.devicePixelRatio});
	})()`)
	if err != nil {
		return rect{}, err
	}
	var r rect
	if err := json.Unmarshal([]byte(v), &r); err != nil {
		return rect{}, fmt.Errorf("the page did not say where its canvas is (%q): %w", v, err)
	}
	if r.W < 1 || r.H < 1 {
		return rect{}, fmt.Errorf("the canvas is %gx%g, which is not on the screen", r.W, r.H)
	}
	return r, nil
}

// ribbon is the band the editor's commands sit in: the top of the canvas,
// inset so that neither its border nor the rail down its left is counted.
func (r rect) ribbon() image.Rectangle {
	x := int(r.X * r.D)
	y := int(r.Y * r.D)
	w := int(r.W * r.D)
	h := int(r.H * r.D)
	return image.Rect(x+w/4, y+h/40, x+3*w/4, y+h/10)
}

func mean(img image.Image, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	var sum float64
	var n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			sum += float64(cr>>8) + float64(cg>>8) + float64(cb>>8)
			n += 3
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// flat reports whether every pixel in r is the same colour — a page that has
// not painted yet, which would otherwise read as a perfectly stable theme.
func flat(img image.Image, r image.Rectangle) bool {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return true
	}
	first := img.At(r.Min.X, r.Min.Y)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.At(x, y) != first {
				return false
			}
		}
	}
	return true
}
