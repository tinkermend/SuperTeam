use std::path::PathBuf;

use superteam_runtime_agent::providers::ProviderRequest;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
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
