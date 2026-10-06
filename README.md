# go-pdfkit landing

Hugo source for **<https://go-pdfkit.github.io/>** — the landing page of the
[`go-pdfkit`](https://github.com/go-pdfkit) org.

Preview locally:

```
hugo server
```

Build:

```
hugo --minify
```

The rendered site lives under `public/` (gitignored except when GitHub Pages is
deployed from that directory — see `.github/workflows/deploy-pages.yml`).

## The layout is the `go-*` family one

`layouts/partials/styles.html` is the whole design system,
`layouts/partials/theme-toggle.html` the three-way theme toggle
(system / light / dark, defaulting to system), and `layouts/index.html` the
page: nav, hero with a terminal, a strip naming the libraries, sections, and a
`.grid` of repo cards drawn from `[[params.repos]]` in `hugo.toml`.

That stylesheet is **byte-identical** to the one `go-authn`, `go-fileshare` and
`go-pkgx` carry. Adding a sibling repository means one `[[params.repos]]` block
and nothing else: the counts on the page are derived with
`{{ len .Site.Params.repos }}` rather than typed, because a number written
beside the data it counts is right the day it is written and has no way of
staying right.

## The colours come from `[params.brand]`

Both stops are **read off `static/img/logo.svg`**, not chosen: `#EF4444 →
#991B1B`. The darker stop is the light-mode accent, because it has to carry
text on white; the brighter one is the dark-mode accent; `deep` is a further
step down for the gradient's far end.

The chip tints are the bright stop mixed into white at 12 % and 30 % — the same
derivation the amber family uses, and reproducing go-pkgx's committed `#fef3e2`
and `#fce2b6` from its `#F59E0B` is how that rule was confirmed rather than
assumed.

Contrast is measured, not eyeballed (WCAG 2.1; the bar for body text is 4.5:1,
and an accent carries links inside paragraphs, so that is the bar that applies):

| | |
|---|---|
| `#991B1B` on `#ffffff` | **8.31:1** AAA |
| `#EF4444` on `#0b0e14` | **5.13:1** AA |
| `#7F1D1D` on `#fde9e9` | **8.59:1** AAA |
| `#fca5a5` on `#0b0e14` | **10.18:1** AAA |

## The stylesheet's defaults are the cyan family's, and that is checked

Every default in `styles.html` is the cyan `go-authn` and `go-fileshare` use, so
those two can carry this exact file with no `[params.brand]` at all and render
what they render today. It is checked rather than claimed:

```sh
# build go-authn as it ships, and this site with [params.brand] commented out
hugo --destination /tmp/ref  --source ../go-authn.github.io
hugo --destination /tmp/ours
# then compare the emitted <style> blocks — they must be identical
```

⛔ Four values used to be written into the **rules** instead of taken as
parameters — the nav glyph's drop shadow and three hero washes — so every org
using this sheet glowed cyan whatever its own logo was. Measured on the
published amber page before the fix: `go-pkgx`'s rendered CSS carried four cyan
literals. They are now `glow`, `wash` and `washDeep`, and each default is the
**exact text** that was in the rule, so the check above still compares equal.

## License

BSD-3-Clause.
