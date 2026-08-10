//! Golden tests: contracts/provider/golden/** → parser events (Phase 2).
//! Failures are semantic fixtures, not dumps of a buggy parser.

use std::fs;
use std::path::PathBuf;

use serde::Deserialize;
use serde_json::Value;
use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::parse_claude_event;
use superteam_runtime_agent::providers::codex::parse_codex_event;
use superteam_runtime_agent::providers::opencode::parse_opencode_event;

#[derive(Debug, Deserialize)]
struct GoldenCase {
    #[allow(dead_code)]
    description: Option<String>,
    native_lines: Vec<String>,
    #[serde(default)]
    expected_events: Vec<Value>,
    #[serde(default)]
    expect_error: bool,
    #[serde(default)]
    error_contains: Option<String>,
}

fn golden_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../contracts/provider/golden")
}

fn load_cases(provider: &str) -> Vec<(PathBuf, GoldenCase)> {
    let dir = golden_root().join(provider);
    let mut out = Vec::new();
    let entries = fs::read_dir(&dir).unwrap_or_else(|e| panic!("read {}: {e}", dir.display()));
    for entry in entries {
        let entry = entry.expect("dir entry");
        let path = entry.path();
        if path.extension().and_then(|e| e.to_str()) != Some("json") {
            continue;
        }
        let raw = fs::read_to_string(&path).expect("read golden");
        let case: GoldenCase = serde_json::from_str(&raw)
            .unwrap_or_else(|e| panic!("parse {}: {e}", path.display()));
        out.push((path, case));
    }
    out.sort_by(|a, b| a.0.cmp(&b.0));
    out
}

fn parse_line(
    provider: &str,
    line: &str,
) -> Result<Vec<ProviderEvent>, anyhow::Error> {
    match provider {
        "claude-code" => parse_claude_event(line),
        "opencode" => parse_opencode_event(line),
        "codex" => parse_codex_event(line),
        other => panic!("unknown provider {other}"),
    }
}

fn assert_event_matches(got: &ProviderEvent, expected: &Value, path: &str) {
    let got_v = serde_json::to_value(got).expect("serialize event");
    let exp_type = expected
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    let got_type = got_v
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    assert_eq!(
        got_type, exp_type,
        "{path}: type mismatch got={got_v} expected={expected}"
    );

    if let Some(obj) = expected.as_object() {
        for (key, exp_val) in obj {
            if key == "type" {
                continue;
            }
            if key == "error" {
                let got_err = got_v
                    .get("error")
                    .expect("expected error object on event");
                if let Some(err_obj) = exp_val.as_object() {
                    for (ek, ev) in err_obj {
                        assert_eq!(
                            got_err.get(ek),
                            Some(ev),
                            "{path}: error.{ek} mismatch got={got_err} expected={exp_val}"
                        );
                    }
                }
                continue;
            }
            assert_eq!(
                got_v.get(key),
                Some(exp_val),
                "{path}: field {key} mismatch got={got_v} expected={expected}"
            );
        }
    }
}

fn run_provider_goldens(provider: &str) {
    let cases = load_cases(provider);
    assert!(
        !cases.is_empty(),
        "no golden cases for {provider} under {}",
        golden_root().join(provider).display()
    );
    // Design target: ≥5 cases per provider (incl. ≥1 failure / unmapped path).
    assert!(
        cases.len() >= 5,
        "{provider}: expected ≥5 golden cases, got {} under {}",
        cases.len(),
        golden_root().join(provider).display()
    );
    for (path, case) in cases {
        let label = path.display().to_string();
        if case.expect_error {
            let mut saw_err = false;
            for line in &case.native_lines {
                match parse_line(provider, line) {
                    Ok(_) => {}
                    Err(err) => {
                        saw_err = true;
                        if let Some(needle) = &case.error_contains {
                            let msg = err.to_string();
                            assert!(
                                msg.contains(needle),
                                "{label}: error {msg:?} missing {needle:?}"
                            );
                        }
                    }
                }
            }
            assert!(saw_err, "{label}: expected parser error");
            continue;
        }

        let mut events = Vec::new();
        for line in &case.native_lines {
            let batch = parse_line(provider, line)
                .unwrap_or_else(|e| panic!("{label}: parse failed: {e:#}"));
            events.extend(batch);
        }
        assert_eq!(
            events.len(),
            case.expected_events.len(),
            "{label}: event count mismatch got={events:?}"
        );
        for (i, (got, exp)) in events.iter().zip(case.expected_events.iter()).enumerate() {
            assert_event_matches(got, exp, &format!("{label}[{i}]"));
        }
    }
}

#[test]
fn golden_claude_code() {
    run_provider_goldens("claude-code");
}

#[test]
fn golden_opencode() {
    run_provider_goldens("opencode");
}

#[test]
fn golden_codex() {
    run_provider_goldens("codex");
}
