# Provider golden samples

Native stdout lines → expected platform events.

## Rules

1. Samples are **semantically derived** from provider docs/fields, not dumped
   from a buggy parser (spec §7.5).
2. Each provider directory needs ≥1 failure sample.
3. Desensitize secrets; keep structure realistic.
4. Runtime tests under `apps/runtime-agent/tests/provider_golden_test.rs` load
   these files and assert deep equality (partial fields for error envelopes).

## Layout

```
claude-code/*.json
opencode/*.json
codex/*.json
```

Each file:

```json
{
  "description": "...",
  "native_lines": ["{...}", "..."],
  "expected_events": [{ "type": "text_delta", "text": "..." }],
  "expect_error": false,
  "error_contains": "optional when expect_error"
}
```
