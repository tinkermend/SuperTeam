use superteam_runtime_agent::instances::{EnsureInstanceRequest, ensure_instance};

#[test]
fn ensure_instance_creates_agent_home_directories() {
    let temp = tempfile::tempdir().expect("tempdir");
    let execution_instance_id = "11111111-1111-4111-8111-111111111111";
    let result = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        execution_instance_id: execution_instance_id.to_string(),
    })
    .expect("ensure instance");

    assert!(
        result
            .agent_home_dir
            .ends_with(format!("agents/{execution_instance_id}"))
    );
    assert!(result.agent_home_dir.join("state").is_dir());
    assert!(result.agent_home_dir.join("sessions").is_dir());
    assert!(result.agent_home_dir.join("runs").is_dir());
}

#[test]
fn ensure_instance_rejects_invalid_execution_instance_ids() {
    let temp = tempfile::tempdir().expect("tempdir");
    for execution_instance_id in [
        "",
        "instance-1",
        "../outside",
        "11111111-1111-4111-8111-11111111111",
        "11111111-1111-4111-8111-111111111111\n",
        "11111111-1111-4111-8111-11111111111z",
    ] {
        let err = ensure_instance(EnsureInstanceRequest {
            base_dir: temp.path().to_path_buf(),
            execution_instance_id: execution_instance_id.to_string(),
        })
        .expect_err("invalid execution instance id should be rejected");

        assert!(err.to_string().contains("invalid execution instance id"));
    }
}
