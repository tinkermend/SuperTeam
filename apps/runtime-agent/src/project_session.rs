//! 稳定项目目录下的会话能力装卸（spec 2026-07-23 §6）。
//!
//! 会话开始：技能软链 + MCP 旁路投影 + session manifest。
//! 会话结束：按清单 unlink 技能、删除本次投影/清单；禁止删除项目根或业务文件。
//! 宪法/人格不落盘（由 prompt 注入，本模块不写）。
//!
//! 多会话同项目目录竞态已产品接受（§0.9 / §6.3）：不为项目目录加锁；
//! 卸载只拆本 command 清单项，尽力语义，不跨会话串行化。

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result, bail};
use serde::{Deserialize, Serialize};

use crate::commands::payload::RuntimeMCPServerPayload;
use crate::mcp_config::{
    materialize_session_mcp_config, prepare_codex_session_overlay,
};
use crate::project_workspace::{SkillLinkReport, link_provider_skills, shield_projected_capability_paths, unlink_provider_skills};
use crate::workspace_files::atomic_write;

const SESSIONS_DIR: &str = ".superteam/sessions";
const MANIFEST_FILE: &str = "manifest.json";

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ProjectSessionManifest {
    pub command_id: String,
    pub provider_type: String,
    #[serde(default)]
    pub skill_keys: Vec<String>,
    #[serde(default)]
    pub mcp_server_keys: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_projection_path: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub codex_overlay_home: Option<PathBuf>,
}

#[derive(Debug, Clone)]
pub struct ProjectSessionInstall {
    pub manifest: ProjectSessionManifest,
    pub mcp_config_path: Option<PathBuf>,
    pub skill_conflicts: Vec<String>,
    /// Provider 进程环境覆盖（不改宿主 auth home；仅会话 overlay）。
    pub provider_overlay_env: BTreeMap<String, String>,
}

pub fn session_dir(workspace_path: &Path, command_id: &str) -> Result<PathBuf> {
    let command_id = command_id.trim();
    if command_id.is_empty() {
        bail!("command_id is required for project session");
    }
    if command_id.contains('/') || command_id.contains('\\') || command_id.contains("..") {
        bail!("command_id is not a safe path segment");
    }
    Ok(workspace_path.join(SESSIONS_DIR).join(command_id))
}

pub fn manifest_path(workspace_path: &Path, command_id: &str) -> Result<PathBuf> {
    Ok(session_dir(workspace_path, command_id)?.join(MANIFEST_FILE))
}

/// 会话开始：先收敛本 command 残留，再装载技能/MCP 并写清单。
pub fn install_project_session(
    agent_home_dir: &Path,
    workspace_path: &Path,
    command_id: &str,
    provider_type: &str,
    skill_keys: &[String],
    mcp_servers: &[RuntimeMCPServerPayload],
) -> Result<ProjectSessionInstall> {
    // 崩溃恢复：同 command_id 残留先卸干净，再装载。
    unload_project_session(workspace_path, command_id)
        .with_context(|| format!("unload residual project session before install: {command_id}"))?;

    match install_project_session_inner(
        agent_home_dir,
        workspace_path,
        command_id,
        provider_type,
        skill_keys,
        mcp_servers,
    ) {
        Ok(install) => Ok(install),
        Err(error) => {
            // 半截装载：按请求 skill keys 尽力 unlink（项目原生非软链跳过），
            // 并清掉 session 目录，避免稳定目录残留。
            let _ = unlink_provider_skills(workspace_path, provider_type, skill_keys);
            let _ = unload_project_session(workspace_path, command_id);
            Err(error)
        }
    }
}

fn install_project_session_inner(
    agent_home_dir: &Path,
    workspace_path: &Path,
    command_id: &str,
    provider_type: &str,
    skill_keys: &[String],
    mcp_servers: &[RuntimeMCPServerPayload],
) -> Result<ProjectSessionInstall> {
    let mcp_config_path =
        materialize_session_mcp_config(workspace_path, command_id, provider_type, mcp_servers)?;
    let mcp_server_keys: Vec<String> = mcp_servers
        .iter()
        .map(|server| server.server_key.clone())
        .collect();

    let mut provider_overlay_env = BTreeMap::new();
    let mut codex_overlay_home = None;

    match provider_type {
        "codex" if mcp_config_path.is_some() => {
            let overlay = session_dir(workspace_path, command_id)?.join("codex-home");
            prepare_codex_session_overlay(&overlay, mcp_servers)?;
            provider_overlay_env.insert(
                "CODEX_HOME".to_string(),
                overlay.to_string_lossy().to_string(),
            );
            codex_overlay_home = Some(overlay);
        }
        "opencode" => {
            if let Some(path) = &mcp_config_path {
                provider_overlay_env.insert(
                    "OPENCODE_CONFIG".to_string(),
                    path.to_string_lossy().to_string(),
                );
            }
        }
        _ => {}
    }

    let link_report: SkillLinkReport =
        link_provider_skills(agent_home_dir, workspace_path, provider_type, skill_keys)?;
    // §7: projected skill symlinks + .superteam/mcp must not pollute user git status.
    shield_projected_capability_paths(workspace_path, provider_type, &link_report.linked)?;

    let manifest = ProjectSessionManifest {
        command_id: command_id.to_string(),
        provider_type: provider_type.to_string(),
        skill_keys: link_report.linked.clone(),
        mcp_server_keys,
        mcp_projection_path: mcp_config_path.clone(),
        codex_overlay_home,
    };
    write_manifest(workspace_path, &manifest)?;

    Ok(ProjectSessionInstall {
        mcp_config_path,
        skill_conflicts: link_report.skipped,
        provider_overlay_env,
        manifest,
    })
}

/// 按清单卸载：unlink 技能软链、删除本次 session 目录（含 MCP 投影与 manifest）。
/// 不存在清单时为 no-op。绝不删除项目根。
pub fn unload_project_session(workspace_path: &Path, command_id: &str) -> Result<()> {
    let mpath = match manifest_path(workspace_path, command_id) {
        Ok(path) => path,
        Err(_) => return Ok(()),
    };
    if !mpath.exists() {
        // 无清单：仍尝试清掉空 session 目录残留。
        let dir = session_dir(workspace_path, command_id)?;
        if dir.exists() {
            let _ = fs::remove_dir_all(&dir);
        }
        return Ok(());
    }

    let raw = fs::read_to_string(&mpath)
        .with_context(|| format!("read project session manifest {}", mpath.display()))?;
    let manifest: ProjectSessionManifest = serde_json::from_str(&raw)
        .with_context(|| format!("parse project session manifest {}", mpath.display()))?;

    unlink_provider_skills(
        workspace_path,
        &manifest.provider_type,
        &manifest.skill_keys,
    )?;

    let dir = session_dir(workspace_path, command_id)?;
    if dir.exists() {
        fs::remove_dir_all(&dir).with_context(|| {
            format!(
                "remove project session dir {} (skills already unlinked)",
                dir.display()
            )
        })?;
    }
    Ok(())
}

pub fn unload_project_session_best_effort(workspace_path: &Path, command_id: &str) {
    if let Err(error) = unload_project_session(workspace_path, command_id) {
        eprintln!(
            "project session unload best-effort failed for command {command_id} at {}: {error:#}",
            workspace_path.display()
        );
    }
}

fn write_manifest(workspace_path: &Path, manifest: &ProjectSessionManifest) -> Result<()> {
    let path = manifest_path(workspace_path, &manifest.command_id)?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("create session manifest dir {}", parent.display()))?;
    }
    let bytes = serde_json::to_vec_pretty(manifest).context("serialize project session manifest")?;
    atomic_write(&path, &bytes)?;
    Ok(())
}

/// 合并 overlay env：不覆盖调用方已显式设置的同名键（保留宿主/派发显式覆盖）。
pub fn merge_provider_overlay_env(
    environment: &mut BTreeMap<String, String>,
    overlay: &BTreeMap<String, String>,
) {
    for (key, value) in overlay {
        environment.entry(key.clone()).or_insert_with(|| value.clone());
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::mcp_config::session_mcp_config_path;
    use tempfile::TempDir;

    fn sample_server(key: &str) -> RuntimeMCPServerPayload {
        RuntimeMCPServerPayload {
            server_id: format!("mcp-{key}"),
            server_key: key.to_string(),
            name: Some(key.to_string()),
            transport: "http".to_string(),
            url: Some(format!("https://example.test/{key}")),
            auth_strategy: None,
            credential_env_var: None,
            required_env_vars: Vec::new(),
            headers_env: BTreeMap::new(),
            source_scope: Some("employee".to_string()),
            config_ref: None,
            permission_scope: serde_json::json!({}),
        }
    }

    #[test]
    fn install_links_skills_writes_manifest_and_unload_cleans() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(home.join(".claude/skills/alpha")).unwrap();
        fs::write(home.join(".claude/skills/alpha/SKILL.md"), "alpha\n").unwrap();
        fs::create_dir_all(&workspace).unwrap();
        fs::write(workspace.join("README.md"), "keep-me\n").unwrap();

        let install = install_project_session(
            &home,
            &workspace,
            "cmd-1",
            "claude-code",
            &["alpha".to_string()],
            &[sample_server("github")],
        )
        .unwrap();

        let skill = workspace.join(".claude/skills/alpha");
        assert!(skill.symlink_metadata().unwrap().file_type().is_symlink());
        assert!(install.mcp_config_path.as_ref().unwrap().exists());
        assert_eq!(install.manifest.skill_keys, vec!["alpha".to_string()]);
        assert_eq!(install.manifest.mcp_server_keys, vec!["github".to_string()]);
        assert!(manifest_path(&workspace, "cmd-1").unwrap().exists());

        unload_project_session(&workspace, "cmd-1").unwrap();

        assert!(!skill.exists());
        assert!(!session_dir(&workspace, "cmd-1").unwrap().exists());
        assert_eq!(
            fs::read_to_string(workspace.join("README.md")).unwrap(),
            "keep-me\n"
        );
    }

    #[test]
    fn unload_does_not_remove_project_native_skill_dir() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(home.join(".claude/skills/alpha")).unwrap();
        fs::write(home.join(".claude/skills/alpha/SKILL.md"), "emp\n").unwrap();
        fs::create_dir_all(home.join(".claude/skills/beta")).unwrap();
        fs::write(home.join(".claude/skills/beta/SKILL.md"), "emp-beta\n").unwrap();
        fs::create_dir_all(workspace.join(".claude/skills/beta")).unwrap();
        fs::write(workspace.join(".claude/skills/beta/SKILL.md"), "native\n").unwrap();

        let install = install_project_session(
            &home,
            &workspace,
            "cmd-2",
            "claude-code",
            &["alpha".to_string(), "beta".to_string()],
            &[],
        )
        .unwrap();
        assert_eq!(install.skill_conflicts, vec!["beta".to_string()]);
        assert_eq!(install.manifest.skill_keys, vec!["alpha".to_string()]);

        unload_project_session(&workspace, "cmd-2").unwrap();
        assert!(!workspace.join(".claude/skills/alpha").exists());
        assert_eq!(
            fs::read_to_string(workspace.join(".claude/skills/beta/SKILL.md")).unwrap(),
            "native\n"
        );
    }

    #[test]
    fn codex_install_sets_overlay_home_env() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(&home).unwrap();
        fs::create_dir_all(&workspace).unwrap();

        let install = install_project_session(
            &home,
            &workspace,
            "cmd-codex",
            "codex",
            &[],
            &[sample_server("docs")],
        )
        .unwrap();

        let overlay = install
            .provider_overlay_env
            .get("CODEX_HOME")
            .expect("CODEX_HOME overlay");
        assert!(Path::new(overlay).join("config.toml").exists());
        let cfg = fs::read_to_string(Path::new(overlay).join("config.toml")).unwrap();
        assert!(cfg.contains("[mcp_servers.docs]"));
        // Host auth is symlinked into overlay when present (process eats MCP
        // without rewriting the real ~/.codex auth home).
        let host_auth = PathBuf::from(std::env::var_os("HOME").unwrap()).join(".codex/auth.json");
        if host_auth.exists() {
            let overlay_auth = Path::new(overlay).join("auth.json");
            assert!(
                overlay_auth
                    .symlink_metadata()
                    .unwrap()
                    .file_type()
                    .is_symlink()
            );
            assert_eq!(fs::read_link(&overlay_auth).unwrap(), host_auth);
        }
        unload_project_session(&workspace, "cmd-codex").unwrap();
        assert!(!Path::new(overlay).exists());
    }

    #[test]
    fn opencode_install_sets_open_config_env() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(&home).unwrap();
        fs::create_dir_all(&workspace).unwrap();

        let install = install_project_session(
            &home,
            &workspace,
            "cmd-oc",
            "opencode",
            &[],
            &[sample_server("docs")],
        )
        .unwrap();

        let cfg = install
            .provider_overlay_env
            .get("OPENCODE_CONFIG")
            .expect("OPENCODE_CONFIG");
        assert!(Path::new(cfg).exists());
        assert!(!install.provider_overlay_env.contains_key("OPENCODE_CONFIG_DIR"));
        unload_project_session(&workspace, "cmd-oc").unwrap();
    }

    #[test]
    fn codex_without_mcp_does_not_set_overlay_home() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(&home).unwrap();
        fs::create_dir_all(&workspace).unwrap();

        let install = install_project_session(&home, &workspace, "cmd-codex-empty", "codex", &[], &[])
            .unwrap();
        assert!(
            !install.provider_overlay_env.contains_key("CODEX_HOME"),
            "empty MCP must not redirect CODEX_HOME (would risk auth/home disruption)"
        );
        assert!(install.mcp_config_path.is_none());
        assert!(install.manifest.codex_overlay_home.is_none());
        unload_project_session(&workspace, "cmd-codex-empty").unwrap();
    }

    #[test]
    fn opencode_without_mcp_does_not_set_open_config() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("proj");
        fs::create_dir_all(&home).unwrap();
        fs::create_dir_all(&workspace).unwrap();

        let install =
            install_project_session(&home, &workspace, "cmd-oc-empty", "opencode", &[], &[])
                .unwrap();
        assert!(!install.provider_overlay_env.contains_key("OPENCODE_CONFIG"));
        assert!(!install.provider_overlay_env.contains_key("OPENCODE_CONFIG_DIR"));
        assert!(install.mcp_config_path.is_none());
        unload_project_session(&workspace, "cmd-oc-empty").unwrap();
    }

    #[test]
    fn merge_overlay_env_does_not_clobber_explicit_keys() {
        let mut env = BTreeMap::from([("CODEX_HOME".to_string(), "/explicit".to_string())]);
        let overlay = BTreeMap::from([
            ("CODEX_HOME".to_string(), "/overlay".to_string()),
            ("OPENCODE_CONFIG".to_string(), "/cfg".to_string()),
        ]);
        merge_provider_overlay_env(&mut env, &overlay);
        assert_eq!(env.get("CODEX_HOME").map(String::as_str), Some("/explicit"));
        assert_eq!(env.get("OPENCODE_CONFIG").map(String::as_str), Some("/cfg"));
    }

    #[test]
    fn session_mcp_path_is_under_sessions_not_shared_mcp() {
        let temp = TempDir::new().unwrap();
        let workspace = temp.path().join("proj");
        fs::create_dir_all(&workspace).unwrap();
        let path = session_mcp_config_path(&workspace, "cmd-x", "claude-code").unwrap();
        assert!(path.ends_with(".superteam/sessions/cmd-x/mcp/claude.mcp.json"));
    }

    #[test]
    fn var_workspaces_install_unload_smoke() {
        if std::env::var_os("SUPERTEAM_P2_VAR_SMOKE").is_none() {
            return;
        }
        let base = PathBuf::from("/var/superteam/workspaces");
        assert!(
            base.exists() && base.is_dir(),
            "/var/superteam/workspaces must exist"
        );
        let probe = base.join(format!(".p2-write-probe-{}", std::process::id()));
        fs::write(&probe, "ok").expect("/var/superteam/workspaces must be writable");
        let _ = fs::remove_file(&probe);

        let home = TempDir::new().unwrap();
        let proj = base.join(format!("p2-api-smoke-{}", std::process::id()));
        let _ = fs::remove_dir_all(&proj);
        fs::create_dir_all(&proj).unwrap();
        fs::write(proj.join("README.md"), "business-keep\n").unwrap();
        fs::create_dir_all(home.path().join(".claude/skills/alpha")).unwrap();
        fs::write(home.path().join(".claude/skills/alpha/SKILL.md"), "alpha\n").unwrap();

        let install = install_project_session(
            home.path(),
            &proj,
            "cmd-var-smoke",
            "claude-code",
            &["alpha".to_string()],
            &[sample_server("github")],
        )
        .unwrap();
        assert!(proj.join(".claude/skills/alpha").is_symlink());
        assert!(install.mcp_config_path.as_ref().unwrap().exists());
        assert!(manifest_path(&proj, "cmd-var-smoke").unwrap().exists());

        unload_project_session(&proj, "cmd-var-smoke").unwrap();
        assert!(!proj.join(".claude/skills/alpha").exists());
        assert!(!session_dir(&proj, "cmd-var-smoke").unwrap().exists());
        assert_eq!(
            fs::read_to_string(proj.join("README.md")).unwrap(),
            "business-keep\n"
        );
        fs::remove_dir_all(&proj).unwrap();
    }
}
