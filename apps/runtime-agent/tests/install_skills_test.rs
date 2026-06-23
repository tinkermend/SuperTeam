use std::path::Path;

use superteam_runtime_agent::commands::install_skills::{provider_skill_dir, validate_skill_key};

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
