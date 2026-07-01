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

    let workspace_path = base_dir
        .join("workspaces")
        .join(project_id)
        .join(task_id)
        .join(attempt_id);
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

fn absolutize_base_dir(base_dir: &Path) -> Result<PathBuf> {
    if base_dir.is_absolute() {
        return Ok(base_dir.to_path_buf());
    }
    Ok(std::env::current_dir()
        .context("resolve current directory for runtime workspace")?
        .join(base_dir))
}

pub fn link_provider_skills(
    agent_home_dir: &Path,
    workspace_path: &Path,
    provider_type: &str,
) -> Result<()> {
    let links: &[(&str, &str)] = match provider_type {
        "claude-code" | "claude" => &[(".claude/skills", ".claude/skills")],
        "codex" => &[(".agents/skills", ".agents/skills")],
        "opencode" => &[(".opencode/skills", ".opencode/skills")],
        _ => &[],
    };

    for (home_rel, workspace_rel) in links {
        let source = agent_home_dir.join(home_rel);
        if !source.exists() {
            continue;
        }

        let target = workspace_path.join(workspace_rel);
        if let Some(parent) = target.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("create skill link parent: {}", parent.display()))?;
        }
        if target.exists() {
            continue;
        }
        #[cfg(unix)]
        std::os::unix::fs::symlink(&source, &target).with_context(|| {
            format!(
                "link provider skills from {} to {}",
                source.display(),
                target.display()
            )
        })?;
    }

    Ok(())
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

fn materialize_git_worktree(
    repo_path: &Path,
    workspace_path: &Path,
    mode: &str,
    base_ref: Option<&str>,
    scope: &[String],
) -> Result<()> {
    if workspace_path.join(".git").exists() {
        apply_sparse_scope(workspace_path, scope)?;
        return Ok(());
    }

    let base = base_ref
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("HEAD");

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
