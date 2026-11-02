// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package playground runs a short, line-per-verb recipe over a PDF and hands
// back the result, so the go-pdfkit verbs can be tried in a browser tab
// against a file that never leaves the machine.
//
// # Why a recipe and not buttons
//
// go-pdfkit/app is already a workbench with buttons. What this adds is the
// thing a playground is for: something you can TYPE, read back, change one
// word of, and send to somebody else as a link. The recipe is the
// shareable artefact; the PDF is the output.
//
// ⛔ THE LANGUAGE IS NOT INVENTED. Every verb is one method of ops.Doc, with
// the same name and the same arguments in the same order, and every page range
// is ops.ParseRange's own spec. A playground whose language drifted from the
// library would teach people something they cannot then write in Go, which is
// the opposite of what it is for.
package playground

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-pdfkit/ops"
)

// MaxSource is how long a recipe may be. It is small on purpose: a recipe is
// a handful of lines, and anything larger is either a mistake or an attempt to
// find out what happens.
const MaxSource = 64 << 10

// MaxPDF is how large an input PDF may be. The whole thing is held in the
// browser's memory, decoded, and held again as pages, so the ceiling is on
// what arrives rather than on what it becomes.
const MaxPDF = 64 << 20

// Result is what one run produced.
type Result struct {
	// PDF is the finished document, empty when the run failed.
	PDF []byte
	// Pages is how many pages it ended with.
	Pages int
	// Log is one line per verb that ran, in order, so a recipe that went
	// wrong half way shows where it got to.
	Log []string
	// Err is empty on success. It names the LINE as well as the trouble: a
	// recipe is a list, and "cannot do that" without saying which line is a
	// message a reader has to bisect by hand.
	Err string
}

// verb is one recipe word and what it does to a document.
type verb struct {
	// args is how many words follow the verb, or -1 for "the rest of the line
	// is one string".
	args int
	// usage is shown when the arguments are wrong, and is also what the
	// playground lists as its help.
	usage string
	run   func(d *ops.Doc, a []string) error
}

// atoi reads an argument as an integer, naming the argument rather than the
// value when it will not parse: "that is not a number" is unhelpful when a
// verb takes three of them.
func atoi(a []string, i int, name string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(a[i]))
	if err != nil {
		return 0, fmt.Errorf("%s should be a whole number, not %q", name, a[i])
	}
	return n, nil
}

func atof(a []string, i int, name string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(a[i]), 64)
	if err != nil {
		return 0, fmt.Errorf("%s should be a number, not %q", name, a[i])
	}
	return f, nil
}

// verbs is the whole language. Adding one is adding a row.
var verbs = map[string]verb{
	"select": {1, "select <pages>", func(d *ops.Doc, a []string) error {
		return d.Select(a[0])
	}},
	"delete": {1, "delete <pages>", func(d *ops.Doc, a []string) error {
		return d.Delete(a[0])
	}},
	"rotate": {2, "rotate <pages> <degrees>", func(d *ops.Doc, a []string) error {
		deg, err := atoi(a, 1, "the angle")
		if err != nil {
			return err
		}
		return d.Rotate(a[0], deg)
	}},
	"reverse": {0, "reverse", func(d *ops.Doc, a []string) error {
		d.Reverse()
		return nil
	}},
	"move": {2, "move <from> <to>", func(d *ops.Doc, a []string) error {
		from, err := atoi(a, 0, "the page to move")
		if err != nil {
			return err
		}
		to, err := atoi(a, 1, "where to move it")
		if err != nil {
			return err
		}
		return d.Move(from, to)
	}},
	"nup": {1, "nup <n>", func(d *ops.Doc, a []string) error {
		n, err := atoi(a, 0, "how many to a sheet")
		if err != nil {
			return err
		}
		return d.NUp(n)
	}},
	"booklet": {0, "booklet", func(d *ops.Doc, a []string) error {
		return d.Booklet()
	}},
	"divide": {2, "divide <across> <down>", func(d *ops.Doc, a []string) error {
		across, err := atoi(a, 0, "how many across")
		if err != nil {
			return err
		}
		down, err := atoi(a, 1, "how many down")
		if err != nil {
			return err
		}
		return d.Divide(across, down)
	}},
	"poster": {2, "poster <across> <down>", func(d *ops.Doc, a []string) error {
		across, err := atoi(a, 0, "how many across")
		if err != nil {
			return err
		}
		down, err := atoi(a, 1, "how many down")
		if err != nil {
			return err
		}
		return d.Poster(across, down)
	}},
	"onepage": {0, "onepage", func(d *ops.Doc, a []string) error {
		return d.OnePage()
	}},
	"insert-blank": {1, "insert-blank <after>", func(d *ops.Doc, a []string) error {
		i, err := atoi(a, 0, "the page to insert after")
		if err != nil {
			return err
		}
		return d.InsertBlank(i)
	}},
	"crop": {5, "crop <pages> <x0> <y0> <x1> <y1>", func(d *ops.Doc, a []string) error {
		var box [4]float64
		for i, name := range []string{"x0", "y0", "x1", "y1"} {
			v, err := atof(a, i+1, name)
			if err != nil {
				return err
			}
			box[i] = v
		}
		return d.Crop(a[0], box)
	}},
	"watermark": {-1, "watermark <pages> <text...>", func(d *ops.Doc, a []string) error {
		return d.Watermark(a[0], a[1])
	}},
	"page-numbers": {-1, "page-numbers <pages> <format...>", func(d *ops.Doc, a []string) error {
		return d.PageNumbers(a[0], a[1])
	}},
	"deskew": {0, "deskew", func(d *ops.Doc, a []string) error {
		_, err := d.Deskew(ops.DeskewOptions{})
		return err
	}},
	"drop-blank": {0, "drop-blank", func(d *ops.Doc, a []string) error {
		_, err := d.DropBlank(ops.BlankOptions{})
		return err
	}},
	"flatten": {0, "flatten", func(d *ops.Doc, a []string) error {
		d.Flatten()
		return nil
	}},
	"sanitize": {0, "sanitize", func(d *ops.Doc, a []string) error {
		d.Sanitize()
		return nil
	}},
	"compress": {0, "compress", func(d *ops.Doc, a []string) error {
		d.Compress()
		return nil
	}},
	"remove-annotations": {0, "remove-annotations", func(d *ops.Doc, a []string) error {
		d.RemoveAnnotations()
		return nil
	}},
	"drop-outlines": {0, "drop-outlines", func(d *ops.Doc, a []string) error {
		d.DropOutlines()
		return nil
	}},
	"clear-info": {0, "clear-info", func(d *ops.Doc, a []string) error {
		d.ClearInfo()
		return nil
	}},
}

// Verbs lists the language, sorted, for the playground's own help panel.
//
// ⛔ Derived from the table rather than typed beside it. A help text typed
// next to the thing it describes is right the day it is typed and has no way
// of staying right.
func Verbs() []string {
	out := make([]string, 0, len(verbs))
	for _, v := range verbs {
		out = append(out, v.usage)
	}
	sort.Strings(out)
	return out
}

// Run applies src to in and returns what came of it.
//
// It never panics on bad input and never returns a half-written PDF: a recipe
// that fails at line four returns the error and the log of lines one to three,
// and no document at all. A viewer handed a partly-applied file would be
// shown something nobody asked for.
func Run(src string, in []byte) Result {
	var r Result
	if len(src) > MaxSource {
		r.Err = fmt.Sprintf("the recipe is %d bytes, past the %d allowed", len(src), MaxSource)
		return r
	}
	if len(in) > MaxPDF {
		r.Err = fmt.Sprintf("the PDF is %d bytes, past the %d allowed", len(in), MaxPDF)
		return r
	}
	if len(in) == 0 {
		r.Err = "there is no PDF to work on yet — open one, or use the example"
		return r
	}

	d, err := ops.Open(in)
	if err != nil {
		r.Err = "that file could not be opened as a PDF: " + err.Error()
		return r
	}
	// ⛔ SAID, NOT SWALLOWED. ops repairs a damaged file rather than refusing
	// it, and a reader who is not told will wonder why the output differs from
	// what another tool shows.
	if d.Repaired() {
		r.Log = append(r.Log, "note: the file was damaged and has been repaired")
	}

	for i, line := range strings.Split(src, "\n") {
		n := i + 1
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := step(d, text); err != nil {
			r.Err = fmt.Sprintf("line %d: %v", n, err)
			return r
		}
		r.Log = append(r.Log, fmt.Sprintf("line %d: %s — %d page(s)", n, text, d.PageCount()))
	}

	out, err := d.Bytes()
	if err != nil {
		r.Err = "the document could not be written: " + err.Error()
		return r
	}
	r.PDF, r.Pages = out, d.PageCount()
	return r
}

// step runs one line.
func step(d *ops.Doc, text string) error {
	name, rest, _ := strings.Cut(text, " ")
	name = strings.ToLower(name)
	v, ok := verbs[name]
	if !ok {
		return fmt.Errorf("there is no verb %q%s", name, nearest(name))
	}

	var a []string
	switch {
	case v.args == -1:
		// The last argument runs to the end of the line, so a watermark can
		// have spaces in it without quoting.
		first, tail, found := strings.Cut(strings.TrimSpace(rest), " ")
		if !found || strings.TrimSpace(tail) == "" {
			return fmt.Errorf("%s takes more than that — %s", name, v.usage)
		}
		a = []string{first, strings.TrimSpace(tail)}
	case v.args == 0:
		if strings.TrimSpace(rest) != "" {
			return fmt.Errorf("%s takes nothing after it — %s", name, v.usage)
		}
	default:
		a = strings.Fields(rest)
		if len(a) != v.args {
			return fmt.Errorf("%s takes %d argument(s), not %d — %s",
				name, v.args, len(a), v.usage)
		}
	}
	return v.run(d, a)
}

// nearest suggests a verb when the one typed is close to a real one.
//
// It exists because the commonest failure in a small language is a plural or
// a hyphen: "pages", "dropblank", "page_numbers". Saying "there is no verb
// X" and stopping leaves the reader to find the list.
func nearest(name string) string {
	best, bestD := "", 3 // further than this is not a typo, it is another word
	for v := range verbs {
		if d := distance(name, v); d < bestD {
			best, bestD = v, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf("; did you mean %q?", best)
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
