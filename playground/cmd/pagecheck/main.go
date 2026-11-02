// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Command pagecheck serves the built site and drives a real browser at the
// playground, then reads the DOM back to see what the page actually produced.
//
// ⛔ IT EXISTS BECAUSE `go test` CANNOT SEE THE PAGE. The recipe engine can be
// perfect while the page shows nothing: a stale wasm_exec.js against a newer
// compiler, a global read before main has set it, a .wasm served as
// octet-stream so instantiateStreaming refuses it. Every one of those leaves
// the Go tests green and the playground blank.
//
// It is the automation of a check that was first done by hand, and it keeps
// the thing that check caught: the page must RENDER, not merely load.
package main

import (
	"flag"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	dir := flag.String("dir", "../public", "the built site")
	chrome := flag.String("chrome", "", "the browser binary")
	budget := flag.Duration("budget", 90*time.Second, "how long the page is given")
	flag.Parse()

	if err := check(*dir, strings.TrimSpace(*chrome), *budget); err != nil {
		fmt.Fprintf(os.Stderr, "pagecheck: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("the playground loaded, ran its example and drew its pages")
}

func check(dir, chrome string, budget time.Duration) error {
	if chrome == "" {
		return fmt.Errorf("no browser given (-chrome)")
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("the built site is not at %s: %w", dir, err)
	}

	// ⛔ application/wasm is set here. Go's mime table does not always know the
	// extension, and a module served as octet-stream makes
	// instantiateStreaming fail with a message about MIME types that reads
	// like a build problem.
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

	url := fmt.Sprintf("http://%s/playground/", ln.Addr().String())
	cmd := exec.Command(chrome,
		"--headless", "--disable-gpu", "--no-sandbox",
		// Virtual time lets the page's timers and the wasm start-up run to
		// completion without this program guessing a sleep.
		fmt.Sprintf("--virtual-time-budget=%d", budget.Milliseconds()),
		"--dump-dom", url)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("the browser failed: %w", err)
	}
	dom := string(out)

	// ⛔ COUNTED BY OCCURRENCE, NOT BY LINE. Minified HTML puts the whole body
	// on one line, so anything that counts matching LINES reports 1 for three
	// images and 1 for thirty. That mistake has been made in this fleet before.
	images := strings.Count(dom, "data:image/png;base64,")
	// The page's own script contains that prefix once, as the string it builds
	// each src from, so one occurrence is the floor and not a drawn page.
	drawn := images - 1

	if strings.Contains(dom, `id="pg-err"`) && !strings.Contains(dom, `id="pg-err" hidden`) {
		return fmt.Errorf("the page is showing an error; DOM:\n%s", excerpt(dom, "pg-err"))
	}
	// The opening example selects six pages and lays them two to a sheet, so
	// three is what it must draw. Asserting the NUMBER and not merely
	// "something appeared" is what makes this a check of the pipeline rather
	// than of the loader.
	const want = 3
	if drawn != want {
		return fmt.Errorf("%d page(s) drawn, want %d — the example is `select 1-6` then `nup 2`;\n%s",
			drawn, want, excerpt(dom, "pg-log"))
	}
	// ⛔ The control on the control. If the DOM came back empty or the page
	// never ran, the count above would be 0 or -1 and the message would say
	// "0 drawn", which reads like a rendering defect rather than like a page
	// that never started. Say which it was.
	if !strings.Contains(dom, "pdfkit") || len(dom) < 4096 {
		return fmt.Errorf("the page does not look like the playground at all (%d bytes)", len(dom))
	}
	return nil
}

// excerpt returns a little of the DOM around a marker, for a failure message
// that can be acted on without re-running anything.
func excerpt(dom, marker string) string {
	i := strings.Index(dom, marker)
	if i < 0 {
		return "(" + marker + " is not in the DOM)"
	}
	end := i + 400
	if end > len(dom) {
		end = len(dom)
	}
	return dom[i:end]
}
