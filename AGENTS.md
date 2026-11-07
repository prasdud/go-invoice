---
description: Behavioral guidelines to reduce common LLM coding mistakes. Use when writing, reviewing, or refactoring code to avoid overcomplication, make surgical changes, surface assumptions, and define verifiable success criteria.
alwaysApply: true
---

# Karpathy behavioral guidelines

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

===

# Invoice Generation Microservice

**Requirements and design live in `openspec/`.** Read `openspec/config.yaml` and
`openspec/changes/trimming-the-fat/` before changing behavior. The original async design
(Redis queue, SQLite audit, status polling, worker pool, admin UI, templates) is superseded
and parked; the proposal lists the triggers that would bring it back.

## What it is now

A synchronous, stateless invoice rendering service:

```
POST /invoices  -> auth -> validate -> semaphore -> render -> upload -> {invoice_id, url}
GET /invoices/{id} -> streams PDF bytes (masked URL; never redirects, never exposes storage)
```

Two engines, selected by the caller per request (never sniffed):

- `basic` — pure Go (gofpdf + embedded DejaVu), milliseconds, default
- `html` — headless Chromium (pooled browser, one tab per render), CJK via system fonts

The only persistence is S3-compatible object storage at `invoices/{id}.pdf`.

## Working rules

- Keep the request flow composed in `internal/api/api.go` (single orchestrator file).
- No Redis, no SQLite, no queue, no polling, no admin UI, no templates. Do not reintroduce
  them without a trigger from the proposal.
- CJK must go through the `html` engine. The basic engine rejects CJK loudly
  (`ErrCJKRequiresHTML`) because DejaVu has no CJK glyphs — never render blank Chinese.
- ID equals storage key: caller-supplied ids are the idempotency mechanism (ULID generated
  when omitted). Keep the `[A-Za-z0-9_-]{22,64}` contract.
- Overload behavior is `503 + Retry-After`, never unbounded goroutines.
- One phase per commit; run `go build ./... && go vet ./... && go test ./...` first.
