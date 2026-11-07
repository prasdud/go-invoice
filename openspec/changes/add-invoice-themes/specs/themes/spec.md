# Delta for Themes

## Purpose

Compile-time invoice themes with deployment branding: the `default` theme reproduces the
current layout; every theme is one Go file backed by an embedded HTML asset; branding
(company name, color, logo) applies without any per-request parsing or lookups.

## ADDED Requirements

### Requirement: Compile-Time Theme Registry

Themes MUST be compiled into the binary, one Go file per theme, each embedding its HTML asset
and registering itself at package init. The registry MUST parse every theme once at process
start and hold immutable templates that are safe for concurrent execution. The service MUST
NOT read, parse, or look up templates per request.

#### Scenario: Startup parsing

- **WHEN** the process starts
- **THEN** every registered theme is parsed exactly once before serving traffic

#### Scenario: Concurrent renders

- **WHEN** multiple renders execute in parallel with the same theme
- **THEN** each produces an independent, valid PDF

#### Scenario: Malformed theme

- **WHEN** a theme's HTML fails to parse
- **THEN** the process refuses to start

### Requirement: Default Theme

The registry MUST provide a theme named `default` that reproduces the established invoice
layout, and html renders through it MUST be behaviorally identical to the previous embedded
template.

#### Scenario: Standard invoice

- **WHEN** a standard payload renders with the html engine
- **THEN** header, meta rows, bill-to block, items, totals, and notes match the established layout

#### Scenario: CJK invoice

- **WHEN** a CJK payload renders with the html engine
- **THEN** all glyphs render as before; themes do not affect font handling

### Requirement: Branding Context

Templates MUST receive `CompanyName`, `PrimaryColor`, an optional `Logo` (prebuilt data URI),
and the invoice data. `COMPANY_NAME` and `COMPANY_COLOR` MUST keep their current behavior;
`COMPANY_LOGO` MUST name a PNG or JPEG file of at most 1 MiB, read and validated once at
startup.

#### Scenario: Logo configured

- **WHEN** `COMPANY_LOGO` points at a valid PNG
- **THEN** the html engine embeds it in rendered invoices
- **AND** the basic engine draws it right-aligned in the header with aspect preserved

#### Scenario: No logo

- **WHEN** `COMPANY_LOGO` is unset
- **THEN** rendered invoices are unchanged and contain no logo

#### Scenario: Invalid logo

- **WHEN** the logo file is missing, not PNG/JPEG, or exceeds 1 MiB
- **THEN** the service refuses to start with a clear error

### Requirement: Offline Template Rendering

Rendered documents MUST NOT reference network or filesystem resources; the logo MUST be
embedded as a data URI. Templates are trusted compile-time artifacts, so no runtime template
validation is required.

#### Scenario: Offline rendering

- **WHEN** a theme renders while the host has no network access
- **THEN** the PDF renders completely, logo included

### Requirement: No Performance Regression

Themes and branding MUST NOT measurably change per-request cost. The renderer benchmarks MUST
stay within noise of the pre-change baseline (basic ~2.5ms, html ~74ms warm on the reference
machine).

#### Scenario: Benchmark parity

- **WHEN** benchmarks run before and after the change on the same machine
- **THEN** per-operation times remain within noise
