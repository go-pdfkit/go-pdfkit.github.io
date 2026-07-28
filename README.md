# go-pdfkit landing

Hugo source for **<https://go-pdfkit.github.io/>** — the landing
page of the [`go-pdfkit`](https://github.com/go-pdfkit) org.

Preview locally:

```
hugo server
```

Build:

```
hugo --minify
```

The rendered site lives under `public/` (gitignored except when
GitHub Pages is deployed from that directory — see the workflow
under `.github/workflows/deploy-pages.yml`).

Layout is a single-page card grid: header + hero + tagline + pills +
a `.grid` of repo cards driven by `hugo.toml`'s `params.repos` list.
Adding a new sibling repo means adding one `[[params.repos]]` block
to `hugo.toml` and re-running `hugo --minify`.

## License

BSD-3-Clause.
