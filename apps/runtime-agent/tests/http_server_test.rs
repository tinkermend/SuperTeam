use std::fs;
use std::os::unix::fs::PermissionsExt;

use reqwest::StatusCode;
use serde_json::json;
use superteam_runtime_agent::server::{RuntimeHttpConfig, RuntimeHttpServer};
use tempfile::TempDir;

fn make_script(dir: &TempDir, name: &str, body: &str) -> std::path::PathBuf {
    let path = dir.path().join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

#[tokio::test]
async fn http_server_creates_run_and_replays_events() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"http-session"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello over http"}]}}'
printf '%s\n' '{"type":"result","result":"http done"}'
"#,
    );

    let server = RuntimeHttpServer::bind_ephemeral(RuntimeHttpConfig {
        node_id: "node-http".to_string(),
        run_log_dir: temp.path().join("run-logs"),
        claude_bin: fake_claude,
        opencode_bin: temp.path().join("missing-opencode"),
    })
    .await
    .expect("bind server");
    let client = reqwest::Client::new();

    let response = client
        .post(format!("http://{}/runs", server.addr()))
        .json(&json!({
            "provider_kind": "claude",
            "workspace_path": temp.path(),
            "prompt": "hello",
            "continue_session": false
        }))
        .send()
        .await
        .expect("post run");
    assert_eq!(response.status(), StatusCode::ACCEPTED);
    let run: serde_json::Value = response.json().await.expect("run json");
    let run_id = run["id"].as_str().expect("run id");

    let mut final_run = serde_json::Value::Null;
    for _ in 0..50 {
        let snapshot: serde_json::Value = client
            .get(format!("http://{}/runs/{run_id}", server.addr()))
            .send()
            .await
            .expect("get run")
            .json()
            .await
            .expect("snapshot json");
        if snapshot["status"] == "completed" {
            final_run = snapshot;
            break;
        }
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }

    assert_eq!(final_run["status"], "completed");
    assert_eq!(final_run["provider_session_id"], "http-session");

    let events: serde_json::Value = client
        .get(format!("http://{}/runs/{run_id}/events", server.addr()))
        .send()
        .await
        .expect("get events")
        .json()
        .await
        .expect("events json");
    let events = events.as_array().expect("events array");
    assert_eq!(events.len(), 3);
    assert_eq!(events[0]["sequence"], 1);
    assert_eq!(events[0]["event"]["type"], "session_started");
    assert_eq!(events[1]["event"]["text"], "hello over http");
    assert_eq!(events[2]["event"]["summary"], "http done");
}

#[tokio::test]
async fn http_server_cancels_active_run() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "slow-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"cancel-session"}'
sleep 5
"#,
    );

    let server = RuntimeHttpServer::bind_ephemeral(RuntimeHttpConfig {
        node_id: "node-http".to_string(),
        run_log_dir: temp.path().join("run-logs"),
        claude_bin: fake_claude,
        opencode_bin: temp.path().join("missing-opencode"),
    })
    .await
    .expect("bind server");
    let client = reqwest::Client::new();

    let run: serde_json::Value = client
        .post(format!("http://{}/runs", server.addr()))
        .json(&json!({
            "provider_kind": "claude",
            "workspace_path": temp.path(),
            "prompt": "hello",
            "continue_session": false
        }))
        .send()
        .await
        .expect("post run")
        .json()
        .await
        .expect("run json");
    let run_id = run["id"].as_str().expect("run id");

    let response = client
        .post(format!("http://{}/runs/{run_id}/cancel", server.addr()))
        .send()
        .await
        .expect("cancel run");
    assert_eq!(response.status(), StatusCode::NO_CONTENT);

    let mut final_run = serde_json::Value::Null;
    for _ in 0..50 {
        let snapshot: serde_json::Value = client
            .get(format!("http://{}/runs/{run_id}", server.addr()))
            .send()
            .await
            .expect("get run")
            .json()
            .await
            .expect("snapshot json");
        if snapshot["status"] == "cancelled" {
            final_run = snapshot;
            break;
        }
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }

    assert_eq!(final_run["status"], "cancelled");
}
