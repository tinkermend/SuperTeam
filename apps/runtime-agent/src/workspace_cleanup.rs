//! 工作区清理闭环（目录与能力投影修订 spec §5 / Phase 3）。
//!
//! 两条路径：
//! - 终态清理：run 结束回执后按 `workspace.cleanup_policy` 处理该 attempt 的
//!   任务工作区（仅 `{base}/workspaces/` 三级 attempt 目录；chat 线程目录有自己
//!   的会话连续性语义，从不在终态清理）。
//! - 后台清扫（janitor）：任务工作区按项目 LRU 裁剪到 `max_retained`；chat 线程
//!   目录按 TTL + 每项目条数兜底清理。活跃 run 引用的目录一律跳过。
//!
//! 稳定项目目录 `{base}/{project_name}`（spec 2026-07-23）**永不**进入本模块的
//! 删除计划：`TerminalWorkspaceCleanup::plan` 只认 legacy
//! `{base}/workspaces/{proj}/{task}/{attempt}` 三段路径。会话装卸由
//! `project_session` 按清单 unlink，禁止靠删项目根清理。
//!
//! 所有删除都是 best-effort：清理失败绝不影响 run 结果，只留 stderr 痕迹。

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

/// 终态清理计划。只对 `{base}/workspaces/{proj}/{task}/{attempt}` 形态的路径
/// 构造得出（其余形态——chat 线程目录、未知路径——构造即返回 None）。
#[derive(Debug, Clone)]
pub struct TerminalWorkspaceCleanup {
    policy: String,
    workspace_path: PathBuf,
    repo_path: Option<PathBuf>,
}

impl TerminalWorkspaceCleanup {
    /// base_dir 必须是绝对路径（调用方经 absolutize 后传入）。
    pub fn plan(policy: &str, base_dir: &Path, workspace_path: &Path) -> Option<Self> {
        let policy = policy.trim().to_ascii_lowercase();
        if policy == "never" || policy.is_empty() {
            return None;
        }
        let project_id = task_attempt_project(base_dir, workspace_path)?;
        let repo_path = base_dir.join("repos").join(project_id);
        Some(Self {
            policy,
            workspace_path: workspace_path.to_path_buf(),
            repo_path: repo_path.join(".git").exists().then_some(repo_path),
        })
    }

    /// 按策略执行。run 成功与否由调用方在终态回写后判定传入。
    pub fn apply(&self, run_succeeded: bool) {
        let should_remove = match self.policy.as_str() {
            "always" => true,
            "on_success" => run_succeeded,
            _ => false,
        };
        if !should_remove {
            return;
        }
        remove_workspace_dir(&self.workspace_path, self.repo_path.as_deref());
    }
}

/// 校验 workspace 是 `{base}/workspaces/{proj}/{task}/{attempt}` 恰好三段的
/// attempt 目录，返回 project 段。这是删除操作的安全闸：不满足即拒绝清理。
fn task_attempt_project<'a>(base_dir: &Path, workspace_path: &'a Path) -> Option<&'a str> {
    let rel = workspace_path.strip_prefix(base_dir.join("workspaces")).ok()?;
    let segments: Vec<&str> = rel.iter().filter_map(|s| s.to_str()).collect();
    if segments.len() != 3
        || segments
            .iter()
            .any(|s| s.is_empty() || *s == "." || *s == "..")
    {
        return None;
    }
    Some(segments[0])
}

fn remove_workspace_dir(workspace_path: &Path, repo_path: Option<&Path>) {
    if !workspace_path.exists() {
        return;
    }
    if let Err(error) = std::fs::remove_dir_all(workspace_path) {
        eprintln!(
            "workspace cleanup failed for {}: {error}",
            workspace_path.display()
        );
        return;
    }
    // worktree 目录被删后仓库缓存里留着悬空注册项，prune 掉;失败无害。
    if let Some(repo_path) = repo_path {
        let _ = std::process::Command::new("git")
            .arg("-C")
            .arg(repo_path)
            .arg("worktree")
            .arg("prune")
            .output();
    }
}

#[derive(Debug, Clone)]
pub struct SweepConfig {
    /// 每项目保留的任务 attempt 工作区数（LRU）。
    pub max_retained_workspaces: usize,
    /// chat 线程目录无活动多久后可删。
    pub chat_ttl: Duration,
    /// 每项目保留的 chat 线程目录数上限（TTL 内也裁剪）。
    pub chat_max_retained: usize,
}

#[derive(Debug, Default, PartialEq, Eq)]
pub struct SweepReport {
    pub removed_workspaces: usize,
    pub removed_chat_threads: usize,
}

/// 后台清扫。`active` 为当前非终态 run 引用的工作区绝对路径集合，一律跳过。
pub fn sweep(base_dir: &Path, config: &SweepConfig, active: &HashSet<PathBuf>) -> SweepReport {
    let mut report = SweepReport::default();
    report.removed_workspaces = sweep_task_workspaces(base_dir, config, active);
    report.removed_chat_threads = sweep_chat_threads(base_dir, config, active);
    report
}

fn sweep_task_workspaces(
    base_dir: &Path,
    config: &SweepConfig,
    active: &HashSet<PathBuf>,
) -> usize {
    let workspaces_root = base_dir.join("workspaces");
    let mut removed = 0usize;
    for project_dir in list_dirs(&workspaces_root) {
        // 收集该项目全部 attempt 目录（task/attempt 两级）。
        let mut attempts: Vec<(SystemTime, PathBuf)> = Vec::new();
        for task_dir in list_dirs(&project_dir) {
            for attempt_dir in list_dirs(&task_dir) {
                attempts.push((last_activity(&attempt_dir), attempt_dir));
            }
        }
        if attempts.len() <= config.max_retained_workspaces {
            continue;
        }
        // 新的在前;超出保留数的最旧目录被裁剪。
        attempts.sort_by(|a, b| b.0.cmp(&a.0));
        let repo_path = project_repo_path(base_dir, &project_dir);
        for (_, attempt_dir) in attempts.iter().skip(config.max_retained_workspaces) {
            if active.contains(attempt_dir) {
                continue;
            }
            remove_workspace_dir(attempt_dir, repo_path.as_deref());
            removed += 1;
            prune_empty_parent(attempt_dir, &project_dir);
        }
    }
    removed
}

fn sweep_chat_threads(base_dir: &Path, config: &SweepConfig, active: &HashSet<PathBuf>) -> usize {
    let chat_root = base_dir.join("chat");
    let now = SystemTime::now();
    let mut removed = 0usize;
    for project_dir in list_dirs(&chat_root) {
        let mut threads: Vec<(SystemTime, PathBuf)> = list_dirs(&project_dir)
            .into_iter()
            .map(|dir| (last_activity(&dir), dir))
            .collect();
        threads.sort_by(|a, b| b.0.cmp(&a.0));
        let repo_path = project_repo_path(base_dir, &project_dir);
        for (index, (activity, thread_dir)) in threads.iter().enumerate() {
            if active.contains(thread_dir) {
                continue;
            }
            let expired = now
                .duration_since(*activity)
                .map(|age| age >= config.chat_ttl)
                .unwrap_or(false);
            let over_cap = index >= config.chat_max_retained;
            if expired || over_cap {
                remove_workspace_dir(thread_dir, repo_path.as_deref());
                removed += 1;
            }
        }
    }
    removed
}

fn project_repo_path(base_dir: &Path, project_dir: &Path) -> Option<PathBuf> {
    let project = project_dir.file_name()?.to_str()?;
    let repo_path = base_dir.join("repos").join(project);
    repo_path.join(".git").exists().then_some(repo_path)
}

/// attempt 目录删掉后,若所在 task 目录已空则一并移除,避免空壳堆积。
fn prune_empty_parent(attempt_dir: &Path, project_dir: &Path) {
    if let Some(parent) = attempt_dir.parent() {
        if parent != project_dir
            && std::fs::read_dir(parent)
                .map(|mut entries| entries.next().is_none())
                .unwrap_or(false)
        {
            let _ = std::fs::remove_dir(parent);
        }
    }
}

fn list_dirs(path: &Path) -> Vec<PathBuf> {
    let Ok(entries) = std::fs::read_dir(path) else {
        return Vec::new();
    };
    entries
        .filter_map(|entry| entry.ok())
        .filter(|entry| {
            entry
                .file_type()
                .map(|kind| kind.is_dir())
                .unwrap_or(false)
        })
        .map(|entry| entry.path())
        .collect()
}

/// 活动时间 = 目录自身与一级子项 mtime 的最大值。深层写入不必然更新根目录
/// mtime,取一级深度在准确性与扫描成本间折中;误差方向是"早删",由 TTL 缺省
/// 值(天级)吸收。
fn last_activity(dir: &Path) -> SystemTime {
    let own = std::fs::metadata(dir)
        .and_then(|meta| meta.modified())
        .unwrap_or(SystemTime::UNIX_EPOCH);
    let Ok(entries) = std::fs::read_dir(dir) else {
        return own;
    };
    entries
        .filter_map(|entry| entry.ok())
        .filter_map(|entry| entry.metadata().ok())
        .filter_map(|meta| meta.modified().ok())
        .fold(own, |acc, item| acc.max(item))
}

/// 后台清扫循环:启动 60s 后首扫,此后每 30 分钟一轮。跳过活跃 run 引用的
/// 目录;所有删除 best-effort。挂在命令循环所在任务旁,进程存活即持续。
pub fn spawn_janitor(config: crate::config::RuntimeConfig, runs: crate::runs::RuntimeRunStore) {
    let Ok(base_dir) =
        crate::project_workspace::absolutize_workspace_base_dir(&config.workspace_base_dir())
    else {
        eprintln!("workspace janitor disabled: cannot absolutize workspace base dir");
        return;
    };
    let sweep_config = SweepConfig {
        max_retained_workspaces: config.workspace.max_retained as usize,
        chat_ttl: Duration::from_secs(u64::from(config.workspace.chat_ttl_days) * 86_400),
        chat_max_retained: config.workspace.chat_max_retained as usize,
    };
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_secs(60)).await;
        loop {
            let active = runs.active_workspaces().await;
            let report = tokio::task::spawn_blocking({
                let base_dir = base_dir.clone();
                let sweep_config = sweep_config.clone();
                move || sweep(&base_dir, &sweep_config, &active)
            })
            .await
            .unwrap_or_default();
            if report.removed_workspaces > 0 || report.removed_chat_threads > 0 {
                eprintln!(
                    "workspace janitor: removed {} task workspaces, {} chat threads",
                    report.removed_workspaces, report.removed_chat_threads
                );
            }
            tokio::time::sleep(Duration::from_secs(30 * 60)).await;
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn mk(path: &Path) {
        std::fs::create_dir_all(path).unwrap();
    }

    fn set_mtime(path: &Path, age: Duration) {
        let stamp = SystemTime::now() - age;
        let times = std::fs::FileTimes::new().set_modified(stamp);
        let dir = std::fs::File::open(path).unwrap();
        dir.set_times(times).unwrap();
    }

    #[test]
    fn terminal_plan_rejects_chat_and_foreign_paths() {
        let temp = TempDir::new().unwrap();
        let base = temp.path();
        let chat = base.join("chat/p1/t1");
        mk(&chat);
        assert!(TerminalWorkspaceCleanup::plan("on_success", base, &chat).is_none());
        assert!(TerminalWorkspaceCleanup::plan("on_success", base, Path::new("/tmp/x")).is_none());
        // Stable project dirs are never terminal-cleaned (P2 / spec 2026-07-23).
        let stable = base.join("my-stable-project");
        assert!(
            TerminalWorkspaceCleanup::plan("always", base, &stable).is_none(),
            "stable {{base}}/{{project_name}} must not enter attempt cleanup"
        );
        let shallow = base.join("workspaces/p1");
        mk(&shallow);
        assert!(TerminalWorkspaceCleanup::plan("on_success", base, &shallow).is_none());
        assert!(
            TerminalWorkspaceCleanup::plan(
                "never",
                base,
                &base.join("workspaces/p1/t1/a1")
            )
            .is_none()
        );
    }

    #[test]
    fn on_success_removes_only_successful_runs_workspace() {
        let temp = TempDir::new().unwrap();
        let base = temp.path();
        let ws = base.join("workspaces/p1/t1/a1");
        mk(&ws);
        std::fs::write(ws.join("out.txt"), "x").unwrap();

        let plan = TerminalWorkspaceCleanup::plan("on_success", base, &ws).unwrap();
        plan.apply(false);
        assert!(ws.exists(), "failed run keeps workspace for debugging");
        plan.apply(true);
        assert!(!ws.exists(), "successful run workspace removed");
    }

    #[test]
    fn always_policy_removes_failed_runs_workspace() {
        let temp = TempDir::new().unwrap();
        let base = temp.path();
        let ws = base.join("workspaces/p1/t1/a1");
        mk(&ws);
        let plan = TerminalWorkspaceCleanup::plan("always", base, &ws).unwrap();
        plan.apply(false);
        assert!(!ws.exists());
    }

    #[test]
    fn sweep_prunes_oldest_task_workspaces_beyond_retention_and_skips_active() {
        let temp = TempDir::new().unwrap();
        let base = temp.path();
        for (name, age_days) in [("a1", 5u64), ("a2", 4), ("a3", 3), ("a4", 2), ("a5", 1)] {
            let dir = base.join("workspaces/p1/t1").join(name);
            mk(&dir);
            set_mtime(&dir, Duration::from_secs(age_days * 86_400));
        }
        let active: HashSet<PathBuf> = [base.join("workspaces/p1/t1/a1")].into();
        let report = sweep(
            base,
            &SweepConfig {
                max_retained_workspaces: 2,
                chat_ttl: Duration::from_secs(7 * 86_400),
                chat_max_retained: 20,
            },
            &active,
        );

        // a5/a4 最新保留;a3/a2 被裁;a1 最旧但活跃,跳过。
        assert_eq!(report.removed_workspaces, 2);
        assert!(base.join("workspaces/p1/t1/a5").exists());
        assert!(base.join("workspaces/p1/t1/a4").exists());
        assert!(!base.join("workspaces/p1/t1/a3").exists());
        assert!(!base.join("workspaces/p1/t1/a2").exists());
        assert!(base.join("workspaces/p1/t1/a1").exists());
    }

    #[test]
    fn sweep_removes_expired_and_over_cap_chat_threads() {
        let temp = TempDir::new().unwrap();
        let base = temp.path();
        let fresh = base.join("chat/p1/fresh");
        let stale = base.join("chat/p1/stale");
        mk(&fresh);
        mk(&stale);
        set_mtime(&stale, Duration::from_secs(8 * 86_400));

        let report = sweep(
            base,
            &SweepConfig {
                max_retained_workspaces: 10,
                chat_ttl: Duration::from_secs(7 * 86_400),
                chat_max_retained: 20,
            },
            &HashSet::new(),
        );
        assert_eq!(report.removed_chat_threads, 1);
        assert!(fresh.exists());
        assert!(!stale.exists());

        // 条数兜底:TTL 内也只保留最新 chat_max_retained 个。
        for name in ["t1", "t2", "t3"] {
            mk(&base.join("chat/p2").join(name));
        }
        let report = sweep(
            base,
            &SweepConfig {
                max_retained_workspaces: 10,
                chat_ttl: Duration::from_secs(7 * 86_400),
                chat_max_retained: 2,
            },
            &HashSet::new(),
        );
        assert_eq!(report.removed_chat_threads, 1);
        assert_eq!(list_dirs(&base.join("chat/p2")).len(), 2);
    }

    #[test]
    fn last_activity_sees_one_level_deep_writes() {
        let temp = TempDir::new().unwrap();
        let dir = temp.path().join("thread");
        mk(&dir);
        set_mtime(&dir, Duration::from_secs(10 * 86_400));
        std::fs::write(dir.join("recent.txt"), "x").unwrap();
        let age = SystemTime::now()
            .duration_since(last_activity(&dir))
            .unwrap();
        assert!(age < Duration::from_secs(60), "child write counts as activity");
    }
}
