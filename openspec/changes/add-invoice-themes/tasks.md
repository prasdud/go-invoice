# Tasks

## 1. Theme Package

- [ ] 1.1 Add `internal/renderer/templates` with `registry.go`: `Theme` type, registry, `Default()`, shared FuncMap (`f2s`).
- [ ] 1.2 Move `invoice.html` to `templates/default.html`; add `templates/default.go` (`//go:embed`, `init()` registration, parse with the FuncMap).
- [ ] 1.3 Switch the html renderer to execute `templates.Default()`; remove the old embed and template from `chromedp.go`.
  - Verify: `go build ./... && go test ./...`; rendered html invoices unchanged in fields and layout.

## 2. Branding (Logo)

- [ ] 2.1 Config: `COMPANY_LOGO` (optional path); load at startup, sniff PNG/JPEG by content, cap 1 MiB, fail fast on invalid.
- [ ] 2.2 `renderer.Options`: logo bytes, type, and prebuilt data URI; html context gains `Logo` (`template.URL`); `default.html` header shows it when present.
- [ ] 2.3 Basic engine: draw the logo right-aligned in the header, aspect preserved, fitting header height; no logo means layout unchanged.
- [ ] 2.4 `.env.example`: document `COMPANY_LOGO`.
  - Verify: tests for present / absent / invalid logo; visual check of both engines with a sample logo.

## 3. Docs

- [ ] 3.1 README: "Themes & branding" section — new env var plus how to add a theme (one Go file + one HTML asset).
- [ ] 3.2 AGENTS.md working rule: themes are compile-time; one Go file per theme; never load templates at runtime.
  - Verify: docs review against the spec.

## 4. Validation

- [ ] 4.1 Benchmarks before/after within noise (`go test ./internal/renderer -bench . -run '^$'`).
- [ ] 4.2 OpenSpec validation (strict) plus `go build ./...`, `go vet ./...`, `go test ./...`.
