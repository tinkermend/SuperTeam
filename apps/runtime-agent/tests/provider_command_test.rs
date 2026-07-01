use std::collections::BTreeMap;
use std::path::PathBuf;

use superteam_runtime_agent::providers::ProviderRequest;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::codex::CodexProvider;
use superteam_runtime_agent::providers::opencode::OpenCodeProvider;

fn request(session_id: Option<&str>, continue_session: bool) -> ProviderRequest {
    ProviderRequest {
        prompt: "hello".to_string(),
        workspace_path: PathBuf::from("/tmp/workspace"),
        agent_home_dir: None,
        employee_capability_dir: None,
        capability_manifest_version: None,
        provider_auth_mode: "host".to_string(),
        mcp_config_path: None,
        session_id: session_id.map(ToString::to_string),
        continue_session,
        model: Some("model-a".to_string()),
        environment: BTreeMap::from([("GH_TOKEN".to_string(), "plain-token".to_string())]),
    }
}

fn request_with_employee_capability_dir(employee_capability_dir: PathBuf) -> ProviderRequest {
    ProviderRequest {
        agent_home_dir: Some(employee_capability_dir.clone()),
        employee_capability_dir: Some(employee_capability_dir),
        ..request(None, false)
    }
}

fn request_with_employee_capability_dir_and_env(
    employee_capability_dir: PathBuf,
    environment: BTreeMap<String, String>,
) -> ProviderRequest {
    ProviderRequest {
        agent_home_dir: Some(employee_capability_dir.clone()),
        employee_capability_dir: Some(employee_capability_dir),
        environment,
        ..request(None, false)
    }
}

fn request_with_task_mcp_config(mcp_config_path: PathBuf) -> ProviderRequest {
    ProviderRequest {
        mcp_config_path: Some(mcp_config_path),
        ..request(None, false)
    }
}

#[test]
fn claude_new_turn_pins_explicit_session_id() {
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request(Some("session-1"), false));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--session-id", "session-1"])
    );
    assert!(!args.iter().any(|arg| arg == "--resume"));
}

#[test]
fn claude_continue_uses_resume_session_id() {
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request(Some("session-2"), true));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--resume", "session-2"])
    );
}

#[test]
fn claude_uses_runtime_governed_non_interactive_permissions() {
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request(None, false));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--permission-mode", "bypassPermissions"])
    );
}

#[test]
fn providers_inject_runtime_environment() {
    for command in [
        ClaudeProvider::new("claude").build_command(&request(None, false)),
        OpenCodeProvider::new("opencode").build_command(&request(None, false)),
        CodexProvider::new("codex").build_command(&request(None, false)),
    ] {
        let envs: std::collections::HashMap<_, _> = command
            .as_std()
            .get_envs()
            .filter_map(|(key, value)| {
                value.map(|value| {
                    (
                        key.to_string_lossy().to_string(),
                        value.to_string_lossy().to_string(),
                    )
                })
            })
            .collect();
        assert_eq!(
            envs.get("GH_TOKEN").map(String::as_str),
            Some("plain-token")
        );
    }
}

#[test]
fn claude_uses_task_level_mcp_config_when_present() {
    let tmp = tempfile::tempdir().unwrap();
    let mcp_config = tmp
        .path()
        .join(".superteam")
        .join("mcp")
        .join("claude.mcp.json");
    std::fs::create_dir_all(mcp_config.parent().unwrap()).unwrap();
    std::fs::write(&mcp_config, "{}").unwrap();
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request_with_task_mcp_config(mcp_config.clone()));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(args.windows(2).any(|window| {
        window[0] == "--mcp-config" && window[1] == mcp_config.to_string_lossy()
    }));
    assert!(args.iter().any(|arg| arg == "--strict-mcp-config"));
}

#[test]
fn claude_does_not_use_employee_home_mcp_config_without_task_projection() {
    let tmp = tempfile::tempdir().unwrap();
    std::fs::write(tmp.path().join(".mcp.json"), "{}").unwrap();
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request_with_employee_capability_dir(
        tmp.path().to_path_buf(),
    ));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(!args.iter().any(|arg| arg == "--mcp-config"));
    assert!(!args.iter().any(|arg| arg == "--strict-mcp-config"));
}

#[test]
fn codex_uses_workspace_for_cd_without_default_codex_home() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request_with_employee_capability_dir(PathBuf::from(
        "/tmp/agent-home",
    )));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();
    let envs: std::collections::HashMap<_, _> = command
        .as_std()
        .get_envs()
        .filter_map(|(key, value)| {
            value.map(|value| {
                (
                    key.to_string_lossy().to_string(),
                    value.to_string_lossy().to_string(),
                )
            })
        })
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--cd", "/tmp/workspace"])
    );
    assert_eq!(envs.get("CODEX_HOME").map(String::as_str), None);
}

#[test]
fn codex_preserves_explicit_codex_home_environment() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request_with_employee_capability_dir_and_env(
        PathBuf::from("/tmp/agent-home"),
        BTreeMap::from([(
            "CODEX_HOME".to_string(),
            "/tmp/explicit-codex-home".to_string(),
        )]),
    ));
    let envs: std::collections::HashMap<_, _> = command
        .as_std()
        .get_envs()
        .filter_map(|(key, value)| {
            value.map(|value| {
                (
                    key.to_string_lossy().to_string(),
                    value.to_string_lossy().to_string(),
                )
            })
        })
        .collect();

    assert_eq!(
        envs.get("CODEX_HOME").map(String::as_str),
        Some("/tmp/explicit-codex-home")
    );
}

#[test]
fn opencode_uses_workspace_dir_without_default_config_home() {
    let provider = OpenCodeProvider::new("opencode");
    let command = provider.build_command(&request_with_employee_capability_dir(PathBuf::from(
        "/tmp/agent-home",
    )));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();
    let envs: std::collections::HashMap<_, _> = command
        .as_std()
        .get_envs()
        .filter_map(|(key, value)| {
            value.map(|value| {
                (
                    key.to_string_lossy().to_string(),
                    value.to_string_lossy().to_string(),
                )
            })
        })
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--dir", "/tmp/workspace"])
    );
    assert_eq!(envs.get("OPENCODE_CONFIG_DIR").map(String::as_str), None);
    assert_eq!(envs.get("OPENCODE_CONFIG").map(String::as_str), None);
}

#[test]
fn opencode_preserves_explicit_config_environment() {
    let provider = OpenCodeProvider::new("opencode");
    let command = provider.build_command(&request_with_employee_capability_dir_and_env(
        PathBuf::from("/tmp/agent-home"),
        BTreeMap::from([
            (
                "OPENCODE_CONFIG_DIR".to_string(),
                "/tmp/explicit-opencode".to_string(),
            ),
            (
                "OPENCODE_CONFIG".to_string(),
                "/tmp/explicit-opencode/custom.json".to_string(),
            ),
        ]),
    ));
    let envs: std::collections::HashMap<_, _> = command
        .as_std()
        .get_envs()
        .filter_map(|(key, value)| {
            value.map(|value| {
                (
                    key.to_string_lossy().to_string(),
                    value.to_string_lossy().to_string(),
                )
            })
        })
        .collect();

    assert_eq!(
        envs.get("OPENCODE_CONFIG_DIR").map(String::as_str),
        Some("/tmp/explicit-opencode")
    );
    assert_eq!(
        envs.get("OPENCODE_CONFIG").map(String::as_str),
        Some("/tmp/explicit-opencode/custom.json")
    );
}

#[test]
fn opencode_continue_uses_session_flag() {
    let provider = OpenCodeProvider::new("opencode");
    let command = provider.build_command(&request(Some("oc-session"), true));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--session", "oc-session"])
    );
}

#[test]
fn opencode_new_turn_pins_explicit_session_id() {
    let provider = OpenCodeProvider::new("opencode");
    let command = provider.build_command(&request(Some("oc-new-session"), false));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2)
            .any(|window| window == ["--session", "oc-new-session"])
    );
    assert!(!args.iter().any(|arg| arg == "--continue"));
}

#[test]
fn codex_new_turn_uses_runtime_governed_exec_flags() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request(None, false));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert_eq!(args[0], "exec");
    assert!(args.iter().any(|arg| arg == "--json"));
    assert!(
        args.windows(2)
            .any(|window| window == ["--cd", "/tmp/workspace"])
    );
    assert!(
        args.iter()
            .any(|arg| arg == "--dangerously-bypass-approvals-and-sandbox")
    );
    assert!(args.iter().any(|arg| arg == "--skip-git-repo-check"));
    assert!(!args.iter().any(|arg| arg == "--ask-for-approval"));
    assert!(!args.iter().any(|arg| arg == "--sandbox"));
    assert!(
        args.windows(2)
            .any(|window| window == ["--model", "model-a"])
    );
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}

#[test]
fn codex_new_turn_does_not_rebase_relative_workspace_twice() {
    let provider = CodexProvider::new("codex");
    let mut request = request(None, false);
    request.workspace_path = PathBuf::from(".superteam/workspaces/employee");
    let command = provider.build_command(&request);
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert!(
        args.windows(2).any(|window| window == ["--cd", "."]),
        "relative workspace paths should not be passed to --cd after command.current_dir; args: {args:?}"
    );
    assert!(
        !args
            .windows(2)
            .any(|window| window == ["--cd", ".superteam/workspaces/employee"]),
        "relative workspace path would be resolved from inside the workspace and fail"
    );
}

#[test]
fn codex_resume_uses_resume_subcommand_and_bypass_flag() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request(Some("codex-session"), true));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert_eq!(args[0], "exec");
    assert_eq!(args[1], "resume");
    assert_eq!(args[2], "codex-session");
    assert!(args.iter().any(|arg| arg == "--json"));
    assert!(
        args.iter()
            .any(|arg| arg == "--dangerously-bypass-approvals-and-sandbox")
    );
    assert!(args.iter().any(|arg| arg == "--skip-git-repo-check"));
    assert!(
        args.windows(2)
            .any(|window| window == ["--cd", "/tmp/workspace"])
    );
    assert!(
        args.windows(2)
            .any(|window| window == ["--model", "model-a"])
    );
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}

#[test]
fn codex_resume_without_session_uses_last_so_prompt_is_not_session_id() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request(None, true));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert_eq!(args[0], "exec");
    assert_eq!(args[1], "resume");
    assert!(args.iter().any(|arg| arg == "--last"));
    assert!(args.iter().any(|arg| arg == "--json"));
    assert!(
        args.windows(2)
            .any(|window| window == ["--cd", "/tmp/workspace"])
    );
    assert!(args.iter().any(|arg| arg == "--skip-git-repo-check"));
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}
