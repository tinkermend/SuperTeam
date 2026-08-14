use std::process::Command;
use std::time::Duration;

use superteam_runtime_agent::instance_lock::acquire_node_instance_lock_in;

#[test]
fn second_runtime_agent_process_exits_when_node_lock_is_held() {
    let dir = tempfile::TempDir::new().expect("tempdir");
    let held = acquire_node_instance_lock_in("mutex-node", dir.path()).expect("first lock");

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .env("RUNTIME_AGENT_INSTANCE_LOCK_DIR", dir.path())
        .env("HOME", dir.path())
        .args(["--node-id", "mutex-node", "--bootstrap-key", "k"])
        .output()
        .expect("spawn second runtime-agent");

    drop(held);

    assert!(
        !output.status.success(),
        "second instance should exit; stdout={} stderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("already running for node `mutex-node`"),
        "stderr={stderr}"
    );
    assert!(
        stderr.contains("this host allows only one instance per node"),
        "stderr={stderr}"
    );
}

#[test]
fn once_flag_does_not_take_the_node_lock() {
    let dir = tempfile::TempDir::new().expect("tempdir");
    let _held = acquire_node_instance_lock_in("mutex-node", dir.path()).expect("first lock");

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .env("RUNTIME_AGENT_INSTANCE_LOCK_DIR", dir.path())
        .env("HOME", dir.path())
        .args(["--node-id", "mutex-node", "--bootstrap-key", "k", "--once"])
        .output()
        .expect("spawn --once");

    assert!(
        output.status.success(),
        "stderr={}",
        String::from_utf8_lossy(&output.stderr)
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("node=mutex-node"), "stdout={stdout}");
}

#[test]
fn second_instance_exits_before_hanging_on_control_plane() {
    let dir = tempfile::TempDir::new().expect("tempdir");
    let _held = acquire_node_instance_lock_in("mutex-node", dir.path()).expect("first lock");
    let started = std::time::Instant::now();
    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .env("RUNTIME_AGENT_INSTANCE_LOCK_DIR", dir.path())
        .env("HOME", dir.path())
        .args(["--node-id", "mutex-node", "--bootstrap-key", "k"])
        .output()
        .expect("spawn second runtime-agent");
    assert!(started.elapsed() < Duration::from_secs(5));
    assert!(!output.status.success());
}
