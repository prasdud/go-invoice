# Proposal: Add Invoice Themes (Compile-Time, Default Theme)

## Why

Operators want branded invoices — company name, color scheme, and a logo — and eventually
different layouts, without giving up the service's shape: one static binary, zero runtime
template files, and no per-request cost. Runtime templates (files, buckets, caches) would add
surface and speed risk this project deliberately avoided (see `trimming-the-fat`).

Go's `embed` makes a better trade: themes are compiled into the binary, parsed once at
startup, and executed from memory forever after. The current embedded `invoice.html` already
works this way; this change makes it a first-class, extensible concept and adds logo support
to both engines.

## What Changes

- Add an `internal/renderer/templates` package: a compile-time registry where **each theme is
  one Go file** (`//go:embed` of its HTML asset) that registers itself in `init()`. Themes are
  parsed once at process start, immutable afterwards, and safe for concurrent execution.
- Ship one theme, `default`, reproducing today's layout exactly.
- The html renderer executes the `default` theme instead of its own embedded template.
- Add branding: `COMPANY_LOGO` env (PNG/JPEG, max 1 MiB, read once at startup). The html
  engine receives a prebuilt data URI (`{{.Logo}}`); the basic engine draws the logo
  right-aligned in the header. `COMPANY_NAME` / `COMPANY_COLOR` keep their behavior.
- Fail fast at startup on a missing, oversized, or non-PNG/JPEG logo.
- Document how to add a theme (one Go file + one HTML asset) in README and AGENTS.md.

**Explicitly out of scope (parked):**

- Theme selection at runtime (`TEMPLATE` env, per-request `template` field) — arrives with
  the second theme.
- Additional themes, runtime template directories, hot reload.
- SVG logos, remote fonts/assets, any external resource loading from templates.
- Per-tenant or multi-brand layouts.

## Capabilities

### New Capabilities

- `themes`: compile-time theme registry, the `default` theme, branding context (name, color,
  logo), startup validation, and zero-per-request-cost guarantees.

### Modified Capabilities

None.

## Impact

- **Code:** new `internal/renderer/templates` package (`registry.go`, `default.go`,
  `default.html`); `chromedp.go` renders via the registry; `renderer.Options` gains logo
  fields; `internal/config` gains `COMPANY_LOGO`; `cmd/server` loads and validates the logo.
- **Config:** one new optional env var; `.env.example` updated.
- **API:** unchanged.
- **Performance:** unchanged per request (parse-once); the logo adds image bytes to each PDF,
  not render time.
- **Docs:** README theming/branding section; AGENTS.md working rule.
