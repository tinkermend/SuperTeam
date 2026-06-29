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
        session_id: session_id.map(ToString::to_string),
        continue_session,
        model: Some("model-a".to_string()),
        environment: BTreeMap::from([("GH_TOKEN".to_string(), "plain-token".to_string())]),
    }
}

fn request_with_agent_home(agent_home_dir: PathBuf) -> ProviderRequest {
    ProviderRequest {
        agent_home_dir: Some(agent_home_dir),
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
fn claude_uses_agent_home_mcp_config_when_present() {
    let tmp = tempfile::tempdir().unwrap();
    let mcp_config = tmp.path().join(".mcp.json");
    std::fs::write(&mcp_config, "{}").unwrap();
    let provider = ClaudeProvider::new("claude");
    let command = provider.build_command(&request_with_agent_home(tmp.path().to_path_buf()));
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
fn codex_uses_agent_home_for_codex_home_and_workspace_for_cd() {
    let provider = CodexProvider::new("codex");
    let command =
        provider.build_command(&request_with_agent_home(PathBuf::from("/tmp/agent-home")));
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
    assert_eq!(
        envs.get("CODEX_HOME").map(String::as_str),
        Some("/tmp/agent-home/.codex")
    );
}

#[test]
fn opencode_uses_agent_home_config_and_workspace_dir() {
    let provider = OpenCodeProvider::new("opencode");
    let command =
        provider.build_command(&request_with_agent_home(PathBuf::from("/tmp/agent-home")));
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
    assert_eq!(
        envs.get("OPENCODE_CONFIG_DIR").map(String::as_str),
        Some("/tmp/agent-home/.opencode")
    );
    assert_eq!(
        envs.get("OPENCODE_CONFIG").map(String::as_str),
        Some("/tmp/agent-home/.opencode/opencode.json")
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
