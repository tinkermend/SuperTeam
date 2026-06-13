use superteam_runtime_agent::instances::{EnsureInstanceRequest, ensure_instance};

#[test]
fn ensure_instance_creates_team_employee_home_without_generic_subdirs() {
    let temp = tempfile::tempdir().unwrap();
    let team_id = "11111111-1111-4111-8111-111111111111";
    let digital_employee_id = "22222222-2222-4222-8222-222222222222";

    let result = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        team_id: team_id.to_string(),
        digital_employee_id: digital_employee_id.to_string(),
    })
    .unwrap();

    assert!(
        result
            .agent_home_dir
            .ends_with(format!("teams/{team_id}/employees/{digital_employee_id}"))
    );
    assert!(result.agent_home_dir.is_dir());
    assert!(!result.agent_home_dir.join("state").exists());
    assert!(!result.agent_home_dir.join("sessions").exists());
    assert!(!result.agent_home_dir.join("runs").exists());
}

#[test]
fn ensure_instance_rejects_invalid_team_and_employee_ids() {
    let temp = tempfile::tempdir().expect("tempdir");
    let valid_team_id = "11111111-1111-4111-8111-111111111111";
    let valid_digital_employee_id = "22222222-2222-4222-8222-222222222222";

    for team_id in [
        "",
        "team-1",
        "../outside",
        "11111111-1111-4111-8111-11111111111",
        "11111111-1111-4111-8111-111111111111\n",
        "11111111-1111-4111-8111-11111111111z",
    ] {
        let err = ensure_instance(EnsureInstanceRequest {
            base_dir: temp.path().to_path_buf(),
            team_id: team_id.to_string(),
            digital_employee_id: valid_digital_employee_id.to_string(),
        })
        .expect_err("invalid team id should be rejected");

        assert!(
            err.to_string()
                .contains("team_id must be a UUID-like string")
        );
    }

    for digital_employee_id in [
        "",
        "employee-1",
        "../outside",
        "22222222-2222-4222-8222-22222222222",
        "22222222-2222-4222-8222-222222222222\n",
        "22222222-2222-4222-8222-22222222222z",
    ] {
        let err = ensure_instance(EnsureInstanceRequest {
            base_dir: temp.path().to_path_buf(),
            team_id: valid_team_id.to_string(),
            digital_employee_id: digital_employee_id.to_string(),
        })
        .expect_err("invalid digital employee id should be rejected");

        assert!(
            err.to_string()
                .contains("digital_employee_id must be a UUID-like string")
        );
    }
}
