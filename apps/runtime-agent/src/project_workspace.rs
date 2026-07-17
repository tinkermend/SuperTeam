use std::ffi::OsString;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{Context, Result};

use crate::commands::payload::RuntimeProjectGitPayload;

#[derive(Debug, Clone)]
pub struct ProjectWorkspaceRequest {
    pub base_dir: PathBuf,
    pub project_id: Option<String>,
    pub project_task_id: Option<String>,
    pub attempt_id: Option<String>,
    /// Set for chat dispatch (目录与能力投影修订 spec §4): the working
    /// directory is then keyed by (project, thread) instead of
    /// (project, task, attempt), so files and provider session state stay
    /// stable across the turns of one conversation.
    pub chat_thread_id: Option<String>,
    /// 派发的 provider 类型。opencode 会在启动时裸加载工作区根部的
    /// opencode.json(c)(实测无官方禁用开关,spec §8.3),物化 worktree 时须
    /// 屏蔽仓库原生 MCP 配置;其余 provider 无此行为。
    pub provider_type: Option<String>,
    pub workspace_mode: String,
    pub project_git: Option<RuntimeProjectGitPayload>,
    pub base_ref: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ResolvedProjectWorkspace {
    pub workspace_path: PathBuf,
    pub repo_path: Option<PathBuf>,
    pub mode: String,
    pub base_ref: Option<String>,
}

pub fn resolve_project_workspace(
    request: ProjectWorkspaceRequest,
) -> Result<ResolvedProjectWorkspace> {
    let mode = normalize_workspace_mode(&request.workspace_mode);
    let base_dir = absolutize_base_dir(&request.base_dir)?;
    let project_id = request.project_id.as_deref().unwrap_or("unscoped");
    let task_id = request.project_task_id.as_deref().unwrap_or("manual");
    let attempt_id = request.attempt_id.as_deref().unwrap_or("attempt");
    validate_segment(project_id, "project_id")?;
    validate_segment(task_id, "project_task_id")?;
    validate_segment(attempt_id, "attempt_id")?;

    if mode != "none" && request.project_git.is_none() {
        anyhow::bail!("project_git is required for workspace_mode={mode}");
    }

    let chat_thread_id = request
        .chat_thread_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let workspace_path = if let Some(thread_id) = chat_thread_id {
        validate_segment(thread_id, "chat_thread_id")?;
        base_dir.join("chat").join(project_id).join(thread_id)
    } else {
        base_dir
            .join("workspaces")
            .join(project_id)
            .join(task_id)
            .join(attempt_id)
    };
    std::fs::create_dir_all(&workspace_path).context("create project task workspace")?;

    let repo_path = if let Some(git) = &request.project_git {
        let repo_path = base_dir.join("repos").join(project_id);
        if mode == "none" {
            ensure_repo_placeholder(&repo_path, git)?;
        } else {
            ensure_repo_cache(&repo_path, git, request.base_ref.as_deref())?;
            materialize_git_worktree(
                &repo_path,
                &workspace_path,
                &mode,
                request.base_ref.as_deref(),
                &git.scope,
                shielded_repo_configs(request.provider_type.as_deref()),
            )?;
        }
        Some(repo_path)
    } else {
        None
    };

    Ok(ResolvedProjectWorkspace {
        workspace_path,
        repo_path,
        mode,
        base_ref: request.base_ref,
    })
}

/// 供工作区清理等旁路调用:与本模块解析工作区时相同的 base_dir 绝对化规则,
/// 保证跨模块的路径前缀比对一致。
pub fn absolutize_workspace_base_dir(base_dir: &Path) -> Result<PathBuf> {
    absolutize_base_dir(base_dir)
}

fn absolutize_base_dir(base_dir: &Path) -> Result<PathBuf> {
    if base_dir.is_absolute() {
        return Ok(base_dir.to_path_buf());
    }
    Ok(std::env::current_dir()
        .context("resolve current directory for runtime workspace")?
        .join(base_dir))
}

/// Links the employee's materialized skills into the task workspace one skill
/// key at a time (目录与能力投影修订 spec §2/§3.1). A key already present in
/// the workspace — e.g. a skill checked into the project repo — wins and the
/// employee-side link is skipped; the skipped keys are returned so callers can
/// surface the conflict instead of hiding it. A missing employee-side source
/// is an error, never a silent no-op: the session payload declared the skill,
/// so the capability cache must contain it by the time linking runs.
pub fn link_provider_skills(
    agent_home_dir: &Path,
    workspace_path: &Path,
    provider_type: &str,
    skill_keys: &[String],
) -> Result<Vec<String>> {
    if skill_keys.is_empty() {
        return Ok(Vec::new());
    }
    let skills_rel = match provider_type {
        "claude-code" | "claude" => ".claude/skills",
        "codex" => ".agents/skills",
        "opencode" => ".opencode/skills",
        other => anyhow::bail!("unsupported provider_type for skill linking: {other}"),
    };

    let mut skipped = Vec::new();
    for key in skill_keys {
        let source = agent_home_dir.join(skills_rel).join(key);
        if !source.exists() {
            anyhow::bail!(
                "employee skill {key} is not materialized at {}",
                source.display()
            );
        }
        let target = workspace_path.join(skills_rel).join(key);
        if let Some(parent) = target.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("create skill link parent: {}", parent.display()))?;
        }
        match std::fs::symlink_metadata(&target) {
            Ok(metadata) => {
                let already_linked = metadata.file_type().is_symlink()
                    && std::fs::read_link(&target)
                        .map(|link| link == source)
                        .unwrap_or(false);
                if already_linked {
                    continue;
                }
                // Project-native content wins (spec §3.1) — skip the
                // employee-side skill, loudly.
                eprintln!(
                    "workspace already provides skill key {key} at {}; employee-side skill skipped (project wins)",
                    target.display()
                );
                skipped.push(key.clone());
            }
            Err(_) => {
                #[cfg(unix)]
                std::os::unix::fs::symlink(&source, &target).with_context(|| {
                    format!(
                        "link provider skill {key} from {} to {}",
                        source.display(),
                        target.display()
                    )
                })?;
            }
        }
    }

    Ok(skipped)
}

fn ensure_repo_placeholder(repo_path: &Path, git: &RuntimeProjectGitPayload) -> Result<()> {
    if git.url.trim().is_empty() {
        anyhow::bail!("project_git.url is required");
    }
    std::fs::create_dir_all(repo_path).context("create project repo cache")?;
    Ok(())
}

fn ensure_repo_cache(
    repo_path: &Path,
    git: &RuntimeProjectGitPayload,
    base_ref: Option<&str>,
) -> Result<()> {
    if git.url.trim().is_empty() {
        anyhow::bail!("project_git.url is required");
    }

    if repo_path.join(".git").exists() {
        run_git(
            repo_path.parent().context("repo parent")?,
            [
                OsString::from("-C"),
                repo_path.as_os_str().to_os_string(),
                OsString::from("fetch"),
                OsString::from("--prune"),
                OsString::from("origin"),
            ],
        )?;
        return Ok(());
    }

    std::fs::create_dir_all(repo_path.parent().context("repo parent")?)?;
    run_git(
        repo_path.parent().context("repo parent")?,
        [
            OsString::from("clone"),
            OsString::from("--no-checkout"),
            OsString::from(git.url.as_str()),
            repo_path.as_os_str().to_os_string(),
        ],
    )?;
    if let Some(base_ref) = base_ref.map(str::trim).filter(|value| !value.is_empty()) {
        run_git(
            repo_path,
            [
                OsString::from("fetch"),
                OsString::from("origin"),
                OsString::from(base_ref),
            ],
        )?;
    }
    Ok(())
}

/// spec §3.2/§8.3:opencode 是三 provider 中唯一会从工作区根裸加载仓库 MCP
/// 配置的(claude 有 --strict-mcp-config,codex 不读项目配置),平台的 MCP 治理
/// 要求注册表是唯一入口,故对 opencode 屏蔽仓库原生配置文件。
fn shielded_repo_configs(provider_type: Option<&str>) -> &'static [&'static str] {
    match provider_type {
        Some("opencode") => &["opencode.json", "opencode.jsonc"],
        _ => &[],
    }
}

fn materialize_git_worktree(
    repo_path: &Path,
    workspace_path: &Path,
    mode: &str,
    base_ref: Option<&str>,
    scope: &[String],
    shielded_configs: &[&str],
) -> Result<()> {
    if workspace_path.join(".git").exists() {
        apply_sparse_scope(workspace_path, scope)?;
        shield_repo_configs(workspace_path, shielded_configs)?;
        return Ok(());
    }

    let base = base_ref
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("HEAD");
    // 仓库缓存的本地分支停留在 clone 时刻;fetch 只推进 origin/*。base_ref 若
    // 直接解析本地分支,repo 绑定项目将永远检出旧代码——优先用 origin/{base}。
    let origin_base = format!("origin/{base}");
    let base = if base != "HEAD"
        && git_ref_exists(repo_path, &format!("refs/remotes/{origin_base}"))
    {
        origin_base
    } else {
        base.to_string()
    };
    let base = base.as_str();

    match mode {
        "branch" => {
            let branch = format!(
                "st/{}/work",
                workspace_path
                    .file_name()
                    .and_then(|value| value.to_str())
                    .unwrap_or("attempt")
            );
            run_git(
                repo_path,
                [
                    OsString::from("worktree"),
                    OsString::from("add"),
                    OsString::from("-B"),
                    OsString::from(branch),
                    workspace_path.as_os_str().to_os_string(),
                    OsString::from(base),
                ],
            )?;
        }
        "detached_run" | "readonly" | "diff" => {
            run_git(
                repo_path,
                [
                    OsString::from("worktree"),
                    OsString::from("add"),
                    OsString::from("--detach"),
                    workspace_path.as_os_str().to_os_string(),
                    OsString::from(base),
                ],
            )?;
        }
        _ => {}
    }

    apply_sparse_scope(workspace_path, scope)?;
    shield_repo_configs(workspace_path, shielded_configs)?;

    Ok(())
}

/// 用 skip-worktree + 删除屏蔽仓库原生配置:文件从工作区消失,而 git
/// status/diff 对该路径保持沉默——不弄脏 worktree,不污染 diff 证据链。
/// 幂等:重放/续聊时文件已不在则跳过。
fn shield_repo_configs(workspace_path: &Path, shielded_configs: &[&str]) -> Result<()> {
    for name in shielded_configs {
        let target = workspace_path.join(name);
        if !target.exists() {
            continue;
        }
        run_git(
            workspace_path,
            [
                OsString::from("update-index"),
                OsString::from("--skip-worktree"),
                OsString::from(*name),
            ],
        )?;
        std::fs::remove_file(&target)
            .with_context(|| format!("shield repo config {}", target.display()))?;
        eprintln!(
            "shielded repo-native provider config {} in {}",
            name,
            workspace_path.display()
        );
    }
    Ok(())
}

fn apply_sparse_scope(workspace_path: &Path, scope: &[String]) -> Result<()> {
    if scope.is_empty() {
        return Ok(());
    }

    run_git(
        workspace_path,
        [
            OsString::from("sparse-checkout"),
            OsString::from("init"),
            OsString::from("--cone"),
        ],
    )?;
    let mut args = vec![OsString::from("sparse-checkout"), OsString::from("set")];
    args.extend(scope.iter().map(OsString::from));
    run_git(workspace_path, args)
}

fn git_ref_exists(repo_path: &Path, refname: &str) -> bool {
    Command::new("git")
        .current_dir(repo_path)
        .args(["rev-parse", "--verify", "--quiet", refname])
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false)
}

fn run_git<I, S>(cwd: &Path, args: I) -> Result<()>
where
    I: IntoIterator<Item = S>,
    S: Into<OsString>,
{
    let args: Vec<OsString> = args.into_iter().map(Into::into).collect();
    let output = Command::new("git")
        .current_dir(cwd)
        .args(&args)
        .output()
        .with_context(|| format!("run git in {}", cwd.display()))?;
    if output.status.success() {
        return Ok(());
    }

    let stderr = String::from_utf8_lossy(&output.stderr);
    let stdout = String::from_utf8_lossy(&output.stdout);
    anyhow::bail!(
        "git command failed in {}: git {}{}{}",
        cwd.display(),
        args.iter()
            .map(|arg| arg.to_string_lossy())
            .collect::<Vec<_>>()
            .join(" "),
        if stderr.trim().is_empty() { "" } else { ": " },
        if stderr.trim().is_empty() {
            stdout.trim()
        } else {
            stderr.trim()
        }
    )
}

fn normalize_workspace_mode(value: &str) -> String {
    match value.trim() {
        "readonly" | "diff" | "detached_run" | "branch" => value.trim().to_string(),
        _ => "none".to_string(),
    }
}

fn validate_segment(value: &str, field: &str) -> Result<()> {
    if value.contains('/')
        || value.contains('\\')
        || value == "."
        || value == ".."
        || value.trim().is_empty()
    {
        anyhow::bail!("{field} is not a safe path segment");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn empty_workspace_when_no_repo_binding() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "none".to_string(),
            project_git: None,
            base_ref: None,
        };

        let resolved = resolve_project_workspace(request).unwrap();

        assert!(resolved.workspace_path.ends_with(
            "workspaces/11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222/33333333-3333-4333-8333-333333333333"
        ));
        assert!(resolved.workspace_path.exists());
        assert_eq!(resolved.mode, "none");
        assert_eq!(resolved.repo_path, None);
    }

    #[test]
    fn rejects_branch_workspace_without_git() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "branch".to_string(),
            project_git: None,
            base_ref: Some("main".to_string()),
        };

        let err = resolve_project_workspace(request).unwrap_err().to_string();

        assert!(err.contains("project_git is required"));
    }

    #[test]
    fn rejects_unsafe_workspace_segments() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("../escape".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "none".to_string(),
            project_git: None,
            base_ref: None,
        };

        let err = resolve_project_workspace(request).unwrap_err().to_string();

        assert!(err.contains("project_id is not a safe path segment"));
    }

    #[test]
    fn materializes_git_worktree_under_project_workspace_and_repo_cache() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(source.join("apps/web")).unwrap();
        std::fs::create_dir_all(source.join("packages/api")).unwrap();
        std::fs::write(source.join("apps/web/README.md"), "web\n").unwrap();
        std::fs::write(source.join("packages/api/README.md"), "api\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "initial"]);

        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().join("runtime"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "readonly".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: vec!["apps/web".to_string()],
            }),
            base_ref: Some("main".to_string()),
        };

        let resolved = resolve_project_workspace(request).unwrap();

        assert!(
            resolved
                .repo_path
                .unwrap()
                .ends_with("repos/11111111-1111-4111-8111-111111111111")
        );
        assert!(resolved.workspace_path.ends_with(
            "workspaces/11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222/33333333-3333-4333-8333-333333333333"
        ));
        assert!(resolved.workspace_path.join("apps/web/README.md").exists());
        assert!(
            !resolved
                .workspace_path
                .join("packages/api/README.md")
                .exists()
        );
    }

    #[test]
    fn materializes_git_worktree_when_base_dir_is_relative() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(&source).unwrap();
        std::fs::write(source.join("README.md"), "hello\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "initial"]);

        let previous_dir = std::env::current_dir().unwrap();
        let runtime_root = temp.path().join("runtime-root");
        std::fs::create_dir_all(&runtime_root).unwrap();
        std::env::set_current_dir(&runtime_root).unwrap();
        let result = resolve_project_workspace(ProjectWorkspaceRequest {
            base_dir: PathBuf::from(".superteam/workspaces"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "readonly".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: Vec::new(),
            }),
            base_ref: Some("main".to_string()),
        });
        std::env::set_current_dir(previous_dir).unwrap();

        let resolved = result.unwrap();
        assert!(resolved.workspace_path.join("README.md").exists());
        assert!(resolved.repo_path.unwrap().is_absolute());
    }

    #[test]
    fn reuses_existing_project_worktree_for_replayed_attempt() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(&source).unwrap();
        std::fs::write(source.join("README.md"), "hello\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "initial"]);

        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().join("runtime"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "branch".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: Vec::new(),
            }),
            base_ref: Some("main".to_string()),
        };

        let first = resolve_project_workspace(request.clone()).unwrap();
        let second = resolve_project_workspace(request).unwrap();

        assert_eq!(second.workspace_path, first.workspace_path);
        assert!(second.workspace_path.join("README.md").exists());
    }

    #[test]
    fn chat_workspace_keyed_by_project_and_thread_is_stable_across_turns() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: None,
            attempt_id: None,
            chat_thread_id: Some("44444444-4444-4444-8444-444444444444".to_string()),
            provider_type: None,
            workspace_mode: "none".to_string(),
            project_git: None,
            base_ref: None,
        };

        let first = resolve_project_workspace(request.clone()).unwrap();
        let second = resolve_project_workspace(request).unwrap();

        assert!(first.workspace_path.ends_with(
            "chat/11111111-1111-4111-8111-111111111111/44444444-4444-4444-8444-444444444444"
        ));
        assert_eq!(second.workspace_path, first.workspace_path);
    }

    #[test]
    fn chat_workspace_rejects_unsafe_thread_segment() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: None,
            attempt_id: None,
            chat_thread_id: Some("../escape".to_string()),
            provider_type: None,
            workspace_mode: "none".to_string(),
            project_git: None,
            base_ref: None,
        };

        let err = resolve_project_workspace(request).unwrap_err().to_string();

        assert!(err.contains("chat_thread_id is not a safe path segment"));
    }

    #[test]
    fn chat_workspace_materializes_readonly_worktree_for_repo_bound_project() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(&source).unwrap();
        std::fs::write(source.join("README.md"), "hello\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "initial"]);

        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().join("runtime"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: None,
            attempt_id: None,
            chat_thread_id: Some("44444444-4444-4444-8444-444444444444".to_string()),
            provider_type: None,
            workspace_mode: "readonly".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: Vec::new(),
            }),
            base_ref: Some("main".to_string()),
        };

        let first = resolve_project_workspace(request.clone()).unwrap();
        let second = resolve_project_workspace(request).unwrap();

        assert!(first.workspace_path.ends_with(
            "chat/11111111-1111-4111-8111-111111111111/44444444-4444-4444-8444-444444444444"
        ));
        assert!(first.workspace_path.join("README.md").exists());
        assert_eq!(second.workspace_path, first.workspace_path);
        assert!(
            first
                .repo_path
                .unwrap()
                .ends_with("repos/11111111-1111-4111-8111-111111111111")
        );
    }

    #[test]
    fn links_declared_skill_keys_and_reports_project_native_conflicts() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("workspace");
        std::fs::create_dir_all(home.join(".claude/skills/alpha")).unwrap();
        std::fs::write(home.join(".claude/skills/alpha/SKILL.md"), "alpha\n").unwrap();
        std::fs::create_dir_all(home.join(".claude/skills/beta")).unwrap();
        std::fs::write(home.join(".claude/skills/beta/SKILL.md"), "beta\n").unwrap();
        // Project repo already ships its own `beta` (project-native skill).
        std::fs::create_dir_all(workspace.join(".claude/skills/beta")).unwrap();
        std::fs::write(
            workspace.join(".claude/skills/beta/SKILL.md"),
            "project-native beta\n",
        )
        .unwrap();

        let skipped = link_provider_skills(
            &home,
            &workspace,
            "claude-code",
            &["alpha".to_string(), "beta".to_string()],
        )
        .unwrap();

        // alpha links from the employee cache; beta stays project-native.
        let alpha = workspace.join(".claude/skills/alpha");
        assert!(alpha.symlink_metadata().unwrap().file_type().is_symlink());
        assert_eq!(
            std::fs::read_link(&alpha).unwrap(),
            home.join(".claude/skills/alpha")
        );
        assert_eq!(skipped, vec!["beta".to_string()]);
        assert_eq!(
            std::fs::read_to_string(workspace.join(".claude/skills/beta/SKILL.md")).unwrap(),
            "project-native beta\n"
        );

        // Replay is idempotent: existing correct links are neither conflicts
        // nor errors.
        let replayed = link_provider_skills(
            &home,
            &workspace,
            "claude-code",
            &["alpha".to_string(), "beta".to_string()],
        )
        .unwrap();
        assert_eq!(replayed, vec!["beta".to_string()]);
    }

    #[test]
    fn linking_a_declared_but_unmaterialized_skill_fails_loudly() {
        let temp = TempDir::new().unwrap();
        let home = temp.path().join("home");
        let workspace = temp.path().join("workspace");
        std::fs::create_dir_all(&home).unwrap();
        std::fs::create_dir_all(&workspace).unwrap();

        let err = link_provider_skills(&home, &workspace, "claude-code", &["ghost".to_string()])
            .unwrap_err()
            .to_string();

        assert!(err.contains("employee skill ghost is not materialized"));
    }

    #[test]
    fn shields_repo_native_opencode_config_without_dirtying_worktree() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(&source).unwrap();
        std::fs::write(source.join("README.md"), "hello\n").unwrap();
        std::fs::write(source.join("opencode.json"), "{\"mcp\":{}}\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "initial"]);

        let request = |provider: Option<&str>, attempt: &str| ProjectWorkspaceRequest {
            base_dir: temp.path().join("runtime"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some(attempt.to_string()),
            chat_thread_id: None,
            provider_type: provider.map(ToString::to_string),
            workspace_mode: "readonly".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: Vec::new(),
            }),
            base_ref: Some("main".to_string()),
        };

        // opencode:仓库原生配置被屏蔽,且 git 状态保持干净(证据链不污染)。
        let opencode = resolve_project_workspace(request(
            Some("opencode"),
            "33333333-3333-4333-8333-333333333333",
        ))
        .unwrap();
        assert!(!opencode.workspace_path.join("opencode.json").exists());
        assert!(opencode.workspace_path.join("README.md").exists());
        let status = std::process::Command::new("git")
            .current_dir(&opencode.workspace_path)
            .args(["status", "--porcelain"])
            .output()
            .unwrap();
        assert!(
            status.stdout.is_empty(),
            "shielding must not dirty the worktree, got: {}",
            String::from_utf8_lossy(&status.stdout)
        );
        // 重放幂等:再次 resolve 不报错,文件保持屏蔽。
        let replay = resolve_project_workspace(request(
            Some("opencode"),
            "33333333-3333-4333-8333-333333333333",
        ))
        .unwrap();
        assert!(!replay.workspace_path.join("opencode.json").exists());

        // 其他 provider 不受影响:项目原生 opencode.json 照常检出。
        let claude = resolve_project_workspace(request(
            Some("claude-code"),
            "44444444-4444-4444-8444-444444444444",
        ))
        .unwrap();
        assert!(claude.workspace_path.join("opencode.json").exists());
    }

    #[test]
    fn new_attempt_checks_out_commits_pushed_after_repo_cache_clone() {
        let temp = TempDir::new().unwrap();
        let source = temp.path().join("source");
        std::fs::create_dir_all(&source).unwrap();
        std::fs::write(source.join("README.md"), "v1\n").unwrap();
        run_test_git(temp.path(), ["init", "-b", "main", "source"]);
        run_test_git(&source, ["config", "user.email", "test@example.com"]);
        run_test_git(&source, ["config", "user.name", "Test User"]);
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "v1"]);

        let request = |attempt: &str| ProjectWorkspaceRequest {
            base_dir: temp.path().join("runtime"),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some(attempt.to_string()),
            chat_thread_id: None,
            provider_type: None,
            workspace_mode: "readonly".to_string(),
            project_git: Some(RuntimeProjectGitPayload {
                url: source.to_string_lossy().to_string(),
                default_branch: Some("main".to_string()),
                git_credential_ref: None,
                scope: Vec::new(),
            }),
            base_ref: Some("main".to_string()),
        };

        // 第一次 attempt 建立仓库缓存。
        resolve_project_workspace(request("33333333-3333-4333-8333-333333333333")).unwrap();
        // 源仓库随后前进一格。
        std::fs::write(source.join("NEW.md"), "v2\n").unwrap();
        run_test_git(&source, ["add", "."]);
        run_test_git(&source, ["commit", "-m", "v2"]);
        // 新 attempt 必须看到新提交(fetch 推进 origin/main,检出用 origin/main
        // 而非停在 clone 时刻的本地 main)。
        let second =
            resolve_project_workspace(request("44444444-4444-4444-8444-444444444444")).unwrap();
        assert!(
            second.workspace_path.join("NEW.md").exists(),
            "new attempt must check out commits pushed after the cache was cloned"
        );
    }

    fn run_test_git<I, S>(cwd: &std::path::Path, args: I)
    where
        I: IntoIterator<Item = S>,
        S: AsRef<std::ffi::OsStr>,
    {
        let output = std::process::Command::new("git")
            .current_dir(cwd)
            .args(args)
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "git failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );
    }
}
