// Copyright (c) the go-pdfkit authors.
// SPDX-License-Identifier: BSD-3-Clause

package playground

import (
	"strings"
	"testing"

	"github.com/go-pdfkit/reader"
)

func example(t *testing.T) []byte {
	t.Helper()
	b, err := ExamplePDF()
	if err != nil {
		t.Fatalf("the example document does not build: %v", err)
	}
	return b
}

// TestTheExampleRunsOnTheExample is the test this package most needs.
//
// ⛔ THE FIRST THING A VISITOR SEES IS THIS RECIPE AGAINST THIS DOCUMENT. If
// it errors, the lesson they take away is that the tool is broken, and they
// will not stay to find out whose fault it was. Every other test here could
// pass with that one failing.
func TestTheExampleRunsOnTheExample(t *testing.T) {
	r := Run(ExampleRecipe, example(t))
	if r.Err != "" {
		t.Fatalf("the opening example fails: %s", r.Err)
	}
	if len(r.PDF) == 0 {
		t.Fatal("the opening example produced no document")
	}
	if r.Pages == 0 {
		t.Fatal("the opening example produced no pages")
	}
	// ⛔ And the result must be readable by our own reader. A playground that
	// hands the browser bytes no PDF reader accepts has not done its job, and
	// "it produced output" does not notice.
	d, err := reader.Open(r.PDF)
	if err != nil {
		t.Fatalf("the result is not a readable PDF: %v", err)
	}
	if d.PageCount() != r.Pages {
		t.Errorf("the result says %d pages, the reader finds %d", r.Pages, d.PageCount())
	}
}

func TestTheExampleDocumentHasPagesToWorkOn(t *testing.T) {
	d, err := reader.Open(example(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.PageCount() != 8 {
		t.Errorf("%d pages, want 8", d.PageCount())
	}
}

func TestEachVerbDoesSomething(t *testing.T) {
	in := example(t)
	for _, tc := range []struct {
		recipe string
		pages  int // 0 means "do not check"
	}{
		{"select 1-3", 3},
		{"delete 1-4", 4},
		{"reverse", 8},
		{"rotate all 90", 8},
		{"move 1 3", 8},
		{"nup 2", 4},
		{"nup 4", 2},
		{"divide 2 1", 16},
		{"poster 2 1", 16},
		{"onepage", 1},
		{"insert-blank 1", 9},
		{"crop all 0 0 300 400", 8},
		{"flatten", 8},
		{"sanitize", 8},
		{"compress", 8},
		{"remove-annotations", 8},
		{"drop-outlines", 8},
		{"clear-info", 8},
		{"watermark all DRAFT COPY", 8},
		{"deskew", 8},
		{"drop-blank", 0},
		{"booklet", 0},
	} {
		r := Run(tc.recipe, in)
		if r.Err != "" {
			t.Errorf("%q: %s", tc.recipe, r.Err)
			continue
		}
		if len(r.PDF) == 0 {
			t.Errorf("%q: no document came back", tc.recipe)
			continue
		}
		if tc.pages != 0 && r.Pages != tc.pages {
			t.Errorf("%q: %d pages, want %d", tc.recipe, r.Pages, tc.pages)
		}
		if len(r.Log) == 0 {
			t.Errorf("%q: nothing was logged", tc.recipe)
		}
	}
}

// TestEveryVerbInTheHelpIsAVerb: the help panel is generated from the table,
// so this guards the other direction — that each usage line begins with the
// name it documents, which is what a reader will type.
func TestEveryVerbInTheHelpIsAVerb(t *testing.T) {
	for _, usage := range Verbs() {
		name, _, _ := strings.Cut(usage, " ")
		if _, ok := verbs[name]; !ok {
			t.Errorf("the help offers %q, which is not a verb", name)
		}
	}
	if len(Verbs()) != len(verbs) {
		t.Errorf("the help lists %d verbs, the table holds %d", len(Verbs()), len(verbs))
	}
}

func TestCommentsAndBlankLinesAreNotVerbs(t *testing.T) {
	r := Run("\n# a comment\n\n   # an indented one\n\nselect 1\n", example(t))
	if r.Err != "" {
		t.Fatalf("%s", r.Err)
	}
	if len(r.Log) != 1 {
		t.Errorf("%d lines logged, want only the one verb: %v", len(r.Log), r.Log)
	}
}

func TestAnErrorNamesItsLine(t *testing.T) {
	// ⛔ A recipe is a list, and "cannot do that" without a line number is a
	// message the reader has to bisect by hand.
	r := Run("select 1\nreverse\nrotate 1 ninety\n", example(t))
	if r.Err == "" {
		t.Fatal("a bad angle was accepted")
	}
	if !strings.HasPrefix(r.Err, "line 3:") {
		t.Errorf("the error does not name line 3: %q", r.Err)
	}
	// And the work that DID happen is reported, so the reader sees how far it got.
	if len(r.Log) != 2 {
		t.Errorf("%d lines logged before the failure, want 2: %v", len(r.Log), r.Log)
	}
	// ⛔ But no document: a half-applied file is something nobody asked for.
	if len(r.PDF) != 0 {
		t.Error("a failed recipe handed back a partly-applied document")
	}
}

func TestATypoSuggestsTheVerbMeant(t *testing.T) {
	for _, tc := range []struct{ typed, want string }{
		{"reverese", "reverse"},
		{"selct", "select"},
		{"rotat", "rotate"},
		{"compres", "compress"},
	} {
		r := Run(tc.typed+" 1", example(t))
		if r.Err == "" {
			t.Errorf("%q was accepted as a verb", tc.typed)
			continue
		}
		if !strings.Contains(r.Err, tc.want) {
			t.Errorf("%q suggested nothing useful: %s", tc.typed, r.Err)
		}
	}
}

func TestAWordThatIsNotNearAVerbSuggestsNothing(t *testing.T) {
	// ⛔ The control on the suggester. One that always suggests something
	// offers "rotate" for "frobnicate", which is worse than silence: it sends
	// the reader to a verb that cannot do what they asked.
	r := Run("photosynthesise all", example(t))
	if r.Err == "" {
		t.Fatal("accepted")
	}
	if strings.Contains(r.Err, "did you mean") {
		t.Errorf("a word that is not a typo got a suggestion: %s", r.Err)
	}
}

func TestArgumentCountsAreChecked(t *testing.T) {
	// ⛔ The expected usage is spelled out per case rather than probed for a
	// "<". A zero-argument verb's usage is the bare word -- `reverse` -- so a
	// test that looked for an angle bracket would demand a placeholder that
	// must not be there. (It did, and this is the corrected form.)
	for _, tc := range []struct{ why, recipe, usage string }{
		{"too few", "rotate 1", "rotate <pages> <degrees>"},
		{"too many", "rotate 1 90 extra", "rotate <pages> <degrees>"},
		{"something after a verb that takes nothing", "reverse please", "reverse"},
		{"a one-argument verb with none", "select", "select <pages>"},
		{"a rest-of-line verb with no text", "watermark all", "watermark <pages> <text...>"},
		{"a rest-of-line verb with nothing at all", "watermark", "watermark <pages> <text...>"},
	} {
		r := Run(tc.recipe, example(t))
		if r.Err == "" {
			t.Errorf("%s: %q was accepted", tc.why, tc.recipe)
			continue
		}
		// The message must carry the usage, or the reader has to go looking.
		if !strings.Contains(r.Err, tc.usage) {
			t.Errorf("%s: %q said %q, without showing the usage %q",
				tc.why, tc.recipe, r.Err, tc.usage)
		}
	}
}

func TestTheRestOfTheLineIsOneArgument(t *testing.T) {
	// A watermark with spaces in it needs no quoting, which is the whole
	// reason the last argument runs to the end of the line.
	r := Run("watermark all NOT FOR DISTRIBUTION", example(t))
	if r.Err != "" {
		t.Fatalf("%s", r.Err)
	}
	if len(r.PDF) == 0 {
		t.Error("nothing came back")
	}
}

func TestWhatItRefusesBeforeOpeningAnything(t *testing.T) {
	t.Run("no PDF yet", func(t *testing.T) {
		r := Run("reverse", nil)
		if !strings.Contains(r.Err, "no PDF") {
			t.Errorf("%q", r.Err)
		}
	})
	t.Run("a file that is not a PDF", func(t *testing.T) {
		r := Run("reverse", []byte("hello, I am not a PDF"))
		if r.Err == "" {
			t.Error("accepted")
		}
	})
	t.Run("a recipe past the ceiling", func(t *testing.T) {
		r := Run(strings.Repeat("reverse\n", MaxSource), example(t))
		if !strings.Contains(r.Err, "past the") {
			t.Errorf("%q", r.Err)
		}
	})
	t.Run("a PDF past the ceiling", func(t *testing.T) {
		r := Run("reverse", make([]byte, MaxPDF+1))
		if !strings.Contains(r.Err, "past the") {
			t.Errorf("%q", r.Err)
		}
	})
}

func TestAnEmptyRecipeIsTheDocumentUnchanged(t *testing.T) {
	// Not an error: it is how a reader opens a file and looks at it before
	// typing anything.
	in := example(t)
	r := Run("", in)
	if r.Err != "" {
		t.Fatalf("%s", r.Err)
	}
	if r.Pages != 8 {
		t.Errorf("%d pages, want the 8 it came with", r.Pages)
	}
}

func TestTheSpecIsTheLibrarysOwn(t *testing.T) {
	// ⛔ The page ranges are not this package's invention: they are whatever
	// ops.ParseRange accepts. If that ever diverges, a reader who learned the
	// syntax here could not use it in Go, which is the point of the thing.
	for _, spec := range []string{"1", "2-4", "1,3-5", "all", "odd", "even"} {
		if r := Run("select "+spec, example(t)); r.Err != "" {
			t.Errorf("select %s: %s", spec, r.Err)
		}
	}
	if r := Run("select 99-200", example(t)); r.Err == "" {
		t.Error("a range past the end was accepted")
	}
}
