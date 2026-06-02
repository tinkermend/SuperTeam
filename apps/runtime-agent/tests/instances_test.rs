use superteam_runtime_agent::instances::{EnsureInstanceRequest, ensure_instance};

#[test]
fn ensure_instance_creates_agent_home_directories() {
    let temp = tempfile::tempdir().expect("tempdir");
    let result = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        execution_instance_id: "instance-1".to_string(),
    })
    .expect("ensure instance");

    assert!(result.agent_home_dir.ends_with("agents/instance-1"));
    assert!(result.agent_home_dir.join("state").is_dir());
    assert!(result.agent_home_dir.join("sessions").is_dir());
    assert!(result.agent_home_dir.join("runs").is_dir());
}

#[test]
fn ensure_instance_rejects_path_segments() {
    let temp = tempfile::tempdir().expect("tempdir");
    let err = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        execution_instance_id: "../outside".to_string(),
    })
    .expect_err("path traversal should be rejected");

    assert!(err.to_string().contains("invalid execution instance id"));
}
