use std::path::PathBuf;

use superteam_runtime_agent::providers::ProviderRequest;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::codex::CodexProvider;
use superteam_runtime_agent::providers::opencode::OpenCodeProvider;

fn request(session_id: Option<&str>, continue_session: bool) -> ProviderRequest {
    ProviderRequest {
        prompt: "hello".to_string(),
        workspace_path: PathBuf::from("/tmp/workspace"),
        session_id: session_id.map(ToString::to_string),
        continue_session,
        model: Some("model-a".to_string()),
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
    assert!(!args.iter().any(|arg| arg == "--cd"));
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
    assert!(args.iter().any(|arg| arg == "--skip-git-repo-check"));
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}
