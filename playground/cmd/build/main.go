// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Command build compiles the playground to WebAssembly and puts it, with the
// toolchain's own wasm_exec.js, where Hugo will publish it.
//
// ⛔ IT ALSO WEIGHS WHAT IT BUILT. A wasm module is a download before it is a
// program, and the number that matters is the COMPRESSED one, because that is
// what crosses the network. The budget fails the build rather than warning:
// a warning in a CI log is a number nobody reads until the page is slow.
//
//	go run ./cmd/build
//	go run ./cmd/build --max-gzip 8388608
package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// defaultMaxGzip is the budget. Measured 2026-11-02 at 5 942 325 bytes with
// reader, render, extract, ops, convert and pdfkit all linked in, so this
// leaves room for the library to grow without leaving room for it to double.
const defaultMaxGzip = 8 << 20

func main() {
	out := flag.String("out", "../static/playground", "where to write the module")
	maxGzip := flag.Int64("max-gzip", defaultMaxGzip, "fail if the gzipped module is larger than this")
	flag.Parse()

	if err := build(*out, *maxGzip); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		os.Exit(1)
	}
}

func build(out string, maxGzip int64) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	wasm := filepath.Join(out, "playground.wasm")
	cmd := exec.Command("go", "build", "-trimpath", "-o", wasm, "./cmd/playground-wasm")
	// ⛔ CGO_ENABLED=0 is not decoration here: this fleet's promise is pure
	// Go, and a build that quietly linked C would break the one claim the
	// landing page makes above the fold.
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compiling to wasm: %w", err)
	}

	// wasm_exec.js belongs to the toolchain that built the module and is
	// copied from it, never committed: a stale copy against a newer compiler
	// fails at run time with a message about an import nobody recognises.
	if err := copyWasmExec(out); err != nil {
		return err
	}

	n, gz, err := weigh(wasm)
	if err != nil {
		return err
	}
	fmt.Printf("playground.wasm  %d bytes, %d gzipped (%.1f%% of the %d budget)\n",
		n, gz, 100*float64(gz)/float64(maxGzip), maxGzip)
	if gz > maxGzip {
		return fmt.Errorf("the module is %d bytes gzipped, past the %d allowed — "+
			"raise --max-gzip on purpose or find out what was linked in", gz, maxGzip)
	}
	return nil
}

// copyWasmExec puts the toolchain's wasm_exec.js beside the module.
func copyWasmExec(out string) error {
	// Go 1.24 moved it from misc/wasm to lib/wasm. Both are looked for rather
	// than one being assumed, so the build does not break on a toolchain bump
	// with a message about a missing file that IS there, elsewhere.
	root := strings.TrimSpace(runtime.GOROOT())
	if root == "" {
		b, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			return fmt.Errorf("finding GOROOT: %w", err)
		}
		root = strings.TrimSpace(string(b))
	}
	var src string
	for _, c := range []string{
		filepath.Join(root, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(root, "misc", "wasm", "wasm_exec.js"),
	} {
		if _, err := os.Stat(c); err == nil {
			src = c
			break
		}
	}
	if src == "" {
		return fmt.Errorf("no wasm_exec.js under %s in lib/wasm or misc/wasm", root)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "wasm_exec.js"), b, 0o644)
}

// weigh returns the module's size and its gzipped size.
//
// ⛔ Compressed with the encoder's best setting, because that is what a web
// server serving pre-compressed assets will have used. Measuring at the
// default level would report a number nobody will ever see.
func weigh(path string) (int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	c := &counter{}
	zw, err := gzip.NewWriterLevel(c, gzip.BestCompression)
	if err != nil {
		return 0, 0, err
	}
	if _, err := io.Copy(zw, f); err != nil {
		return 0, 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, 0, err
	}
	return st.Size(), c.n, nil
}

// counter counts bytes written to it and keeps none of them: the compressed
// module is several megabytes and there is no reason to hold a second copy.
type counter struct{ n int64 }

func (c *counter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
