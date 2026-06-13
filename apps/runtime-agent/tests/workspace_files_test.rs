use superteam_runtime_agent::commands::payload::RuntimeWorkspaceFilePayload;
use superteam_runtime_agent::workspace_files::{
    ProviderHomeKind, WorkspaceMaterializationPlan, materialize_workspace, validate_workspace_path,
};

fn agents_file(content: &str) -> RuntimeWorkspaceFilePayload {
    RuntimeWorkspaceFilePayload {
        file_id: "55555555-5555-4555-8555-555555555555".to_string(),
        revision_id: "66666666-6666-4666-8666-666666666666".to_string(),
        path: "AGENTS.md".to_string(),
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
