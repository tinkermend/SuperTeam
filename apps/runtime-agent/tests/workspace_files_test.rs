use superteam_runtime_agent::workspace_files::{
    ProviderHomeKind, WorkspaceMaterializationPlan, materialize_workspace,
};

// 工作区文件下发功能已下线；materialize_workspace 只负责建立员工家目录的
// provider 私有目录结构（provisioning / 会话前置仍依赖）。

#[test]
fn materialize_workspace_creates_opencode_provider_dir() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("employee");
    std::fs::create_dir_all(&home).unwrap();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::OpenCode,
    })
    .unwrap();

    assert_eq!(result.agent_home_dir, home);
    assert!(home.join(".opencode").is_dir());
}

#[test]
fn materialize_workspace_creates_codex_provider_dir() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("employee");
    std::fs::create_dir_all(&home).unwrap();

    materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::Codex,
    })
    .unwrap();

    assert!(home.join(".codex").is_dir());
    assert!(!home.join("CLAUDE.md").exists());
}

#[cfg(unix)]
#[test]
fn materialize_workspace_rejects_symlink_workspace_root() {
    let temp = tempfile::tempdir().unwrap();
    let real = temp.path().join("real-home");
    std::fs::create_dir_all(&real).unwrap();
    let link = temp.path().join("employee");
    std::os::unix::fs::symlink(&real, &link).unwrap();

    materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: link,
        provider_home: ProviderHomeKind::ClaudeCode,
    })
    .expect_err("symlinked agent home must be rejected");
}
