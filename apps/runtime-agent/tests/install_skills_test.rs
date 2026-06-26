use std::path::Path;

use serde_json::json;
use superteam_runtime_agent::commands::install_skills::{
    InstallSkillsCommandPayload, InstalledSkillTarget, SkillInstallRollback,
    prepare_provider_skill_install_paths, provider_skill_dir, validate_skill_key,
};

#[test]
fn install_skills_provider_skill_dir_maps_official_provider_skill_directories() {
    let agent_home = Path::new("/tmp/superteam/agent-home");

    assert_eq!(
        provider_skill_dir(agent_home, "opencode", "code-review").unwrap(),
        agent_home.join(".opencode/skills/code-review")
    );
    assert_eq!(
        provider_skill_dir(agent_home, "codex", "code-review").unwrap(),
        agent_home.join(".agents/skills/code-review")
    );
    assert_eq!(
        provider_skill_dir(agent_home, "claude-code", "code-review").unwrap(),
        agent_home.join(".claude/skills/code-review")
    );
}

#[test]
fn install_skills_validate_skill_key_rejects_unsafe_keys() {
    for key in ["../review", "bad/key", ""] {
        assert!(
            validate_skill_key(key).is_err(),
            "expected unsafe skill key to fail: {key:?}"
        );
    }
}

#[test]
fn install_skills_provider_skill_dir_rejects_unsupported_provider() {
    let error = provider_skill_dir(Path::new("/tmp/superteam/agent-home"), "unknown", "review")
        .expect_err("unsupported provider should fail");

    assert!(error.to_string().contains("unsupported provider_type"));
}

#[test]
fn install_skills_parses_control_plane_shaped_payload() {
    let payload: InstallSkillsCommandPayload = serde_json::from_value(json!({
        "command_id": "cmd-install",
        "tenant_id": "00000000-0000-4000-8000-000000000001",
        "skill": {
            "skill_id": "11111111-1111-4111-8111-111111111111",
            "skill_key": "code-review",
            "archive_object_ref": "s3://runtime-skills/tenant/code-review.zip",
            "archive_checksum_sha256": "abc123",
            "archive_size_bytes": 2048,
            "archive_file_count": 3
        },
        "targets": [{
            "team_id": "22222222-2222-4222-8222-222222222222",
            "digital_employee_id": "33333333-3333-4333-8333-333333333333",
            "provider_type": "codex",
            "agent_home_dir": "/tmp/superteam/agent-home"
        }],
        "rollback_on_failure": true
    }))
    .expect("control-plane install_skills payload should parse");

    assert_eq!(payload.tenant_id, "00000000-0000-4000-8000-000000000001");
    assert_eq!(payload.skill.skill_key, "code-review");
    assert_eq!(payload.skill.archive_file_count, 3);
    assert!(payload.rollback_on_failure);
    assert_eq!(
        payload.targets[0].team_id,
        "22222222-2222-4222-8222-222222222222"
    );
    assert_eq!(
        payload.targets[0].digital_employee_id,
        "33333333-3333-4333-8333-333333333333"
    );
    assert_eq!(payload.targets[0].provider_type, "codex");
    assert_eq!(
        payload.targets[0].agent_home_dir,
        "/tmp/superteam/agent-home"
    );
}

#[test]
fn install_skills_installed_result_uses_control_plane_field_names() {
    let result = InstalledSkillTarget {
        team_id: "22222222-2222-4222-8222-222222222222".to_string(),
        digital_employee_id: "33333333-3333-4333-8333-333333333333".to_string(),
        agent_home_dir: "/tmp/superteam/agent-home".to_string(),
        provider_type: "codex".to_string(),
        skill_key: "code-review".to_string(),
        installed_path: "/tmp/superteam/agent-home/.agents/skills/code-review".to_string(),
        archive_checksum_sha256: "abc123".to_string(),
        archive_file_count: 3,
    };

    let value = serde_json::to_value(result).expect("serialize installed result");

    assert_eq!(value["team_id"], "22222222-2222-4222-8222-222222222222");
    assert_eq!(
        value["digital_employee_id"],
        "33333333-3333-4333-8333-333333333333"
    );
    assert_eq!(value["provider_type"], "codex");
    assert_eq!(
        value["installed_path"],
        "/tmp/superteam/agent-home/.agents/skills/code-review"
    );
    assert_eq!(value["archive_checksum_sha256"], "abc123");
    assert_eq!(value["archive_file_count"], 3);
    assert!(value.get("path").is_none());
    assert!(value.get("checksum").is_none());
    assert!(value.get("file_count").is_none());
}

#[test]
fn install_skills_rollback_restores_old_dir_and_removes_new_dir() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_home = temp.path().join("agent-home");
    let old_target = agent_home.join(".agents").join("skills").join("old-target");
    let new_target = agent_home.join(".agents").join("skills").join("new-target");
    std::fs::create_dir_all(&old_target).expect("old target dir");
    std::fs::write(old_target.join("SKILL.md"), "old").expect("old file");

    let mut rollback = SkillInstallRollback::new(true, &agent_home).expect("rollback");
    rollback
        .prepare_target(&old_target)
        .expect("backup old target");
    std::fs::create_dir_all(&old_target).expect("new old target dir");
    std::fs::write(old_target.join("SKILL.md"), "new").expect("new old-target file");

    rollback
        .prepare_target(&new_target)
        .expect("track new target");
    std::fs::create_dir_all(&new_target).expect("new target dir");
    std::fs::write(new_target.join("SKILL.md"), "new").expect("new target file");

    rollback.rollback().expect("rollback");

    assert_eq!(
        std::fs::read_to_string(old_target.join("SKILL.md")).expect("restored old file"),
        "old"
    );
    assert!(
        !new_target.exists(),
        "newly-created target should be removed by rollback"
    );
}

#[test]
fn install_skills_rejects_symlinked_provider_parent_escape() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_home = temp.path().join("agent-home");
    let outside = temp.path().join("outside");
    std::fs::create_dir_all(&agent_home).expect("agent home");
    std::fs::create_dir_all(&outside).expect("outside");
    std::os::unix::fs::symlink(&outside, agent_home.join(".agents"))
        .expect("symlink provider root");

    let error = prepare_provider_skill_install_paths(&agent_home, "codex", "code-review")
        .expect_err("symlinked provider root should be rejected");

    assert!(error.to_string().contains("symlink"));
}

#[test]
fn install_skills_rejects_symlinked_skills_parent_escape() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_home = temp.path().join("agent-home");
    let outside = temp.path().join("outside");
    std::fs::create_dir_all(agent_home.join(".agents")).expect("provider root");
    std::fs::create_dir_all(&outside).expect("outside");
    std::os::unix::fs::symlink(&outside, agent_home.join(".agents").join("skills"))
        .expect("symlink skills root");

    let error = prepare_provider_skill_install_paths(&agent_home, "codex", "code-review")
        .expect_err("symlinked skills root should be rejected");

    assert!(error.to_string().contains("symlink"));
}

#[test]
fn install_skills_rollback_uses_owned_root_under_agent_home() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_home = temp.path().join("agent-home");
    let target = agent_home
        .join(".agents")
        .join("skills")
        .join("code-review");
    let rollback_root = agent_home.join(".skill-rollback");
    std::fs::create_dir_all(&target).expect("target dir");
    std::fs::write(target.join("SKILL.md"), "old").expect("old file");

    let mut rollback = SkillInstallRollback::new(true, &agent_home).expect("rollback");
    rollback.prepare_target(&target).expect("backup target");

    assert!(rollback_root.is_dir(), "rollback root should be created");
    let backup_entries = std::fs::read_dir(&rollback_root)
        .expect("rollback root entries")
        .count();
    assert_eq!(backup_entries, 1);
    assert!(!target.exists(), "target should be moved into owned backup");

    rollback.rollback().expect("rollback");

    assert_eq!(
        std::fs::read_to_string(target.join("SKILL.md")).expect("restored file"),
        "old"
    );
    assert!(
        !rollback_root.exists(),
        "rollback should clean its owned root"
    );
}

#[test]
fn install_skills_prepare_paths_creates_missing_agent_home() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_home = temp.path().join("new-agent-home");

    let paths = prepare_provider_skill_install_paths(&agent_home, "codex", "code-review")
        .expect("missing agent home should be created for skill installation");

    assert!(agent_home.is_dir(), "agent home should be created");
    assert_eq!(
        paths.canonical_agent_home,
        agent_home.canonicalize().unwrap()
    );
    assert_eq!(
        paths.target_dir,
        paths
            .canonical_agent_home
            .join(".agents")
            .join("skills")
            .join("code-review")
    );
}
