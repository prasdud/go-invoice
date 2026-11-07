# Design: Add Invoice Themes

## Context

The html engine currently renders one embedded `invoice.html` (`go:embed` in
`internal/renderer/chromedp.go`). The `trimming-the-fat` design parked templates entirely;
this change brings back the minimum viable version: compile-time themes plus branding, with
no runtime template infrastructure.

Constraints carried over:

- One static binary; no runtime file dependencies for templates.
- No per-request template reading, parsing, or cache lookups.
- Single-tenant deployments; branding configured per deployment.
- Fail loudly; never silently degrade.

## Goals / Non-Goals

**Goals:** a first-class theme concept; `default` theme equals current output; logo on both
engines; adding theme #2 is "drop in one Go file"; zero benchmark impact.

**Non-Goals:** runtime selection or serving of themes; a second theme now; SVG logos; hot
reload; template partials across files.

## Decisions

### D1: Compile-time themes instead of runtime files

Immutability replaces caching: parse once at init, hold a `*template.Template` in a package
variable, execute concurrently forever after. Runtime directories or buckets would add a
loader, validation, and failure modes for master copies that live in a repo anyway. The
loader is the shared seam for a future `TEMPLATE_DIR` override if a non-technical self-hoster
ever needs restyling without a rebuild.

### D2: One Go file per theme, self-registering

```
internal/renderer/templates/
  registry.go    Theme type, registry, lookup, shared FuncMap
  default.go     //go:embed default.html + init() registration
  default.html   the layout (stays real HTML for tooling)
```

Adding a theme = add `<name>.go` + `<name>.html`. No central list to edit. The HTML stays a
real file (editor support, `html/template` auto-escaping) rather than a Go string constant.

### D3: Theme contract

```go
type Theme struct {
    Name string
    HTML *template.Template // parsed once at init, immutable
}
```

A malformed theme panics at init, so the process refuses to start — impossible in practice
because themes are compiled and tested. Templates consume the renderer's invoice context
(unchanged fields: `InvoiceNum`, `Items`, ...) plus `CompanyName`, `PrimaryColor`, `Logo`.

### D4: Branding model

- `COMPANY_NAME`, `COMPANY_COLOR` (existing) and `COMPANY_LOGO` (new, optional path).
- The logo is read once at startup, sniffed for PNG/JPEG by content, capped at 1 MiB;
  anything else is a startup error. Raster only: gofpdf cannot draw SVG, and one logo contract
  for both engines is simpler than two.
- html engine: prebuilt `data:` URI passed as `template.URL` (bypasses URL sanitization and
  keeps Chromium offline — no `file://`, no network fetches).
- basic engine: `RegisterImageOptionsReader` + `ImageOptions`, right-aligned in the header
  bar, scaled to header height with aspect preserved.
- Themes MAY hardcode brand values in their own HTML; env values remain the overrides for the
  `default` theme's placeholders.

### D5: Nothing per request

Startup does: read logo → validate → build data URI → registry parses themes. The request
path is unchanged: execute the parsed template, embed bytes. The only per-PDF delta is the
logo's bytes (tens of KB typical), which affects object size, not latency.

## Performance

The existing renderer benchmarks gate this change: the `default` theme must stay within noise
of the baseline (basic ~2.5ms, html ~74ms warm). Template parsing already happened at package
init before this change; the only addition is the per-document image embed.

## Testing

- Registry: `default` loads, parses, and executes with a full context.
- Renderer: existing payload tests pass with unchanged html output; new tests assert the logo
  appears when configured and is absent otherwise; basic-engine header contains the image.
- Config: missing, oversized, and non-image logos are rejected at startup.
- Benchmarks: before/after within noise.

## Parked

Theme selection (`TEMPLATE` env / request field), second theme, `TEMPLATE_DIR` override, hot
reload, SVG logos, runtime template storage (R2), multi-file templates with partials.
