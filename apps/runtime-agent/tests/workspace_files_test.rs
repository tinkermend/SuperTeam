use superteam_runtime_agent::commands::payload::RuntimeWorkspaceFilePayload;
use superteam_runtime_agent::workspace_files::{
    ProviderHomeKind, WorkspaceMaterializationPlan, materialize_workspace, validate_workspace_path,
};

fn agents_file(content: &str) -> RuntimeWorkspaceFilePayload {
    workspace_file("AGENTS.md", content)
}

fn workspace_file(path: &str, content: &str) -> RuntimeWorkspaceFilePayload {
    RuntimeWorkspaceFilePayload {
        file_id: "55555555-5555-4555-8555-555555555555".to_string(),
        revision_id: "66666666-6666-4666-8666-666666666666".to_string(),
        path: path.to_string(),
        file_role: "entrypoint".to_string(),
        mime_type: "text/markdown".to_string(),
        sync_policy: "auto".to_string(),
        content_hash: superteam_runtime_agent::workspace_files::sha256_hex(content.as_bytes()),
        size_bytes: content.len() as i32,
        storage_backend: "db".to_string(),
        content_text: Some(content.to_string()),
        object_key: None,
        metadata: serde_json::json!({}),
    }
}

#[test]
fn rejects_reserved_and_unsafe_workspace_paths() {
    for path in [
        "",
        "/AGENTS.md",
        "../AGENTS.md",
        "notes/../AGENTS.md",
        "CLAUDE.md",
        "CLAUDE.md/anything",
        ".claude/settings.json",
        ".opencode/config.json",
        ".git/config",
        ".superteam/state.json",
    ] {
        assert!(
            validate_workspace_path(path).is_err(),
            "path should be rejected: {path}"
        );
    }
    assert_eq!(
        validate_workspace_path("docs/context.md").unwrap(),
        "docs/context.md"
    );
}

#[test]
fn materialize_workspace_writes_agents_link_and_provider_dir() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    std::fs::create_dir_all(&home).unwrap();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![agents_file("# Contract\n")],
    })
    .unwrap();

    assert_eq!(result.synced_files.len(), 1);
    assert_eq!(
        std::fs::read_to_string(home.join("AGENTS.md")).unwrap(),
        "# Contract\n"
    );
    assert!(home.join(".claude").is_dir());
    assert!(home.join("CLAUDE.md").exists());
    assert!(!home.join("state").exists());
    assert!(!home.join("runs").exists());
}

#[cfg(unix)]
#[test]
fn materialize_workspace_rejects_symlink_parent_escape() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    let outside = temp.path().join("outside");
    std::fs::create_dir_all(&home).unwrap();
    std::fs::create_dir_all(&outside).unwrap();
    std::os::unix::fs::symlink(&outside, home.join("docs")).unwrap();

    let error = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![workspace_file("docs/file.md", "leak\n")],
    })
    .expect_err("symlink parent should be rejected");

    assert!(error.to_string().contains("symlink"));
    assert!(!outside.join("file.md").exists());
}

#[cfg(unix)]
#[test]
fn materialize_workspace_rejects_symlink_file_target() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    let outside = temp.path().join("outside");
    std::fs::create_dir_all(home.join("docs")).unwrap();
    std::fs::create_dir_all(&outside).unwrap();
    std::fs::write(outside.join("target.md"), "outside\n").unwrap();
    std::os::unix::fs::symlink(outside.join("target.md"), home.join("docs/file.md")).unwrap();

    let error = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![workspace_file("docs/file.md", "replacement\n")],
    })
    .expect_err("symlink file target should be rejected");

    assert!(error.to_string().contains("symlink"));
    assert_eq!(
        std::fs::read_to_string(outside.join("target.md")).unwrap(),
        "outside\n"
    );
    assert!(
        std::fs::symlink_metadata(home.join("docs/file.md"))
            .unwrap()
            .file_type()
            .is_symlink()
    );
}

#[test]
fn materialize_workspace_rejects_object_store_files_for_now() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    let mut file = workspace_file("docs/object.md", "");
    file.storage_backend = "object_store".to_string();
    file.content_text = None;
    file.object_key = Some("workspace/docs/object.md".to_string());

    let error = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home,
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![file],
    })
    .expect_err("object_store is not implemented yet");

    assert!(
        error
            .to_string()
            .contains("object_store workspace files are not supported yet")
    );
}

#[test]
fn materialize_workspace_rejects_hash_mismatch() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    let mut file = workspace_file("docs/context.md", "actual\n");
    file.content_hash = superteam_runtime_agent::workspace_files::sha256_hex(b"expected\n");

    let error = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![file],
    })
    .expect_err("content hash mismatch should be rejected");

    assert!(error.to_string().contains("content_hash mismatch"));
    assert!(!home.join("docs/context.md").exists());
}

#[test]
fn materialize_workspace_skips_disabled_sync_files() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    let mut file = workspace_file("docs/disabled.md", "disabled\n");
    file.sync_policy = "disabled".to_string();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::OpenCode,
        files: vec![file],
    })
    .unwrap();

    assert!(result.synced_files.is_empty());
    assert!(!home.join("docs/disabled.md").exists());
    assert!(home.join(".opencode").is_dir());
}
