# Delta for Rendering

## Purpose

Turn a validated invoice payload into PDF bytes on A4 using a caller-selected engine. Two
engines exist: `basic` (pure Go, fast, default) and `html` (headless Chromium, rich layouts
and CJK). Rendering is concurrency-safe and cancellable.

## ADDED Requirements

### Requirement: Engine Contract

Engines MUST be selected from a registry containing exactly `basic` and `html`. A render
call MUST accept a context (for cancellation/timeout), MUST be safe for concurrent use by
multiple goroutines, and MUST return the full PDF bytes or a wrapped error. No engine may
write partial artifacts to storage; upload happens only after a successful render.

#### Scenario: Concurrent renders

- **WHEN** multiple renders execute in parallel
- **THEN** each returns an independent, valid PDF
- **AND** no shared mutable state corrupts the output

#### Scenario: Cancellation

- **WHEN** the render context is cancelled or exceeds its deadline
- **THEN** the engine stops and returns an error
- **AND** the API maps it to `render_timeout`

### Requirement: Caller-Selected Engine

The engine MUST be chosen by the caller via the API's `engine` field. The renderer MUST NOT
inspect payload content to change engines. `basic` is the default.

#### Scenario: No engine switching

- **WHEN** a payload contains CJK text and the caller chose `basic`
- **THEN** the request fails with `422 invalid_payload` and a message directing the caller to
  the `html` engine
- **AND** the service does not silently switch engines or emit glyph-less output

### Requirement: Basic Engine

The basic engine MUST be pure Go (no external processes) and use the embedded font. It
SHOULD render a standard invoice in single-digit milliseconds. It MUST produce A4 output and
MUST render without error when optional fields are missing.

#### Scenario: Standard invoice

- **WHEN** a complete standard payload renders
- **THEN** the PDF contains the header, meta rows, bill-to block, items table, totals, and
  notes

#### Scenario: Long description

- **WHEN** an item description exceeds the column width
- **THEN** it wraps within its cell and the row height grows accordingly

#### Scenario: Missing optional fields

- **WHEN** fields like `due_date` or `notes` are absent
- **THEN** the invoice renders cleanly without placeholder artifacts

#### Scenario: CJK payload rejected

- **WHEN** the payload contains Han, Kana, or Hangul text
- **THEN** the engine returns an error instead of emitting an invoice with missing glyphs

### Requirement: HTML Engine

The html engine MUST render via headless Chromium with the embedded invoice HTML. It MUST
reuse a pooled browser process (MUST NOT launch a browser per render), MUST bound concurrent
pages via the render semaphore, MUST enforce the per-render timeout, and MUST recover by
relaunching the browser after a crash. It MUST render CJK (Han, Kana, Hangul) correctly when
the runtime fonts are present.

#### Scenario: CJK invoice

- **WHEN** a payload with Japanese, Chinese, or Korean text renders with the html engine
- **THEN** all glyphs appear in the PDF (no tofu)

#### Scenario: Browser crash

- **WHEN** the pooled browser dies mid-run
- **THEN** the engine relaunches it
- **AND** subsequent renders succeed without a service restart

#### Scenario: Bounded pages

- **WHEN** concurrent html renders exceed the semaphore size
- **THEN** pages are never created beyond the bound

### Requirement: Payload Contract

The payload MUST be a JSON object. All fields are optional and MUST be recognized exactly as
listed in the scenarios below. Monetary values MUST be computed by the documented formulas,
never taken from unvalidated input.

#### Scenario: Recognized fields

- **WHEN** a payload is parsed
- **THEN** the following fields are recognized:

| Field | Type | Notes |
|---|---|---|
| `invoice_number`, `date`, `due_date` | string | meta rows |
| `bill_to` (or `bill_to_name`), `bill_to_address` | string | address supports `\n` |
| `items` | array | each: `description` (string), `quantity` (number), `rate` (number), optional `amount` (number) |
| `tax_rate` | number | percent |
| `notes` | string | free text |

#### Scenario: Computed totals

- **WHEN** items with quantity and rate render without explicit amounts
- **THEN** the line amount is `quantity x rate`
- **AND** the subtotal is the sum of line amounts
- **AND** the tax is `subtotal x tax_rate / 100`
- **AND** the total is `subtotal + tax`, displayed with an optional tax row

#### Scenario: Explicit amount wins

- **WHEN** an item carries `amount`
- **THEN** that value is used instead of `quantity x rate`

### Requirement: Structural Validation

The renderer input MUST be validated before any engine runs. `items` MUST be an array of
objects; `quantity`, `rate`, `amount`, and `tax_rate` MUST be numeric when present; the item
count MUST be capped (default 200). Violations MUST surface as `invalid_payload` (422) and
MUST NOT reach an engine.

#### Scenario: Invalid items

- **WHEN** `items` is a string or an item's `quantity` is non-numeric
- **THEN** the request fails with `422 invalid_payload`

#### Scenario: Item cap

- **WHEN** a payload contains more items than the cap
- **THEN** the request fails with `422 invalid_payload`

### Requirement: Sanitization and Bounds

Non-BMP characters (emoji) MUST be stripped. String fields MUST be capped (default 5000
characters) before rendering. The html engine MUST escape payload values (no payload-driven
markup or script execution).

#### Scenario: Emoji stripped

- **WHEN** a description contains emoji
- **THEN** the emoji is removed and the remaining text renders

#### Scenario: Injection attempt

- **WHEN** a field contains HTML or script tags and the html engine renders it
- **THEN** the tags appear as literal text and are not executed
