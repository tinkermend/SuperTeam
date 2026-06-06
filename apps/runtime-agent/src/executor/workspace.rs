use anyhow::{Context, Result};
use std::path::PathBuf;
use std::time::Instant;

use crate::config::RuntimeConfig;

pub struct RunWorkspace {
    pub workspace_path: PathBuf,
    pub logs_path: PathBuf,
    pub artifacts_path: PathBuf,
    pub execution_instance_id: String,
    pub run_id: String,
    pub created_at: Instant,
}

impl RunWorkspace {
    pub fn logs_dir(&self) -> &PathBuf {
        &self.logs_path
    }

    pub fn artifacts_dir(&self) -> &PathBuf {
        &self.artifacts_path
    }

    pub fn workspace_dir(&self) -> &PathBuf {
        &self.workspace_path
    }
}

pub fn create_run_workspace(
    config: &RuntimeConfig,
    execution_instance_id: &str,
    run_id: &str,
) -> Result<RunWorkspace> {
    let run_base = config
        .workspace
        .base_dir
        .join("instances")
        .join(execution_instance_id)
        .join("runs")
        .join(run_id);

    let workspace_path = run_base.join("workspace");
    let logs_path = run_base.join("logs");
    let artifacts_path = run_base.join("artifacts");

    std::fs::create_dir_all(&workspace_path).context("Failed to create workspace dir")?;
    std::fs::create_dir_all(&logs_path).context("Failed to create logs dir")?;
    std::fs::create_dir_all(&artifacts_path).context("Failed to create artifacts dir")?;

    Ok(RunWorkspace {
        workspace_path,
        logs_path,
        artifacts_path,
        execution_instance_id: execution_instance_id.to_string(),
        run_id: run_id.to_string(),
        created_at: Instant::now(),
    })
}

#[deprecated(note = "Use create_run_workspace instead")]
pub fn create_task_workspace(config: &RuntimeConfig, task_id: i64) -> Result<TaskWorkspace> {
    let workspace_path = config.workspace.base_dir.join(format!("task-{}", task_id));
    std::fs::create_dir_all(&workspace_path).context("Failed to create task workspace")?;
    Ok(TaskWorkspace {
        path: workspace_path,
        task_id,
        created_at: Instant::now(),
    })
}

pub struct TaskWorkspace {
    pub path: PathBuf,
    pub task_id: i64,
    pub created_at: Instant,
}

pub fn cleanup_workspace(workspace: &TaskWorkspace, config: &RuntimeConfig) -> Result<()> {
    match config.workspace.cleanup_policy.as_str() {
        "on_success" | "on_completion" => {
            remove_workspace(workspace)?;
        }
        "never" => {
            println!("Workspace retained at: {:?}", workspace.path);
        }
        policy => {
            eprintln!(
                "Unknown cleanup policy: {}, defaulting to 'on_completion'",
                policy
            );
            remove_workspace(workspace)?;
        }
    }
    cleanup_old_workspaces(config)?;
    Ok(())
}

pub fn cleanup_run_workspace(workspace: &RunWorkspace, config: &RuntimeConfig) -> Result<()> {
    match config.workspace.cleanup_policy.as_str() {
        "on_success" | "on_completion" => {
            remove_run_workspace(workspace)?;
        }
        "never" => {
            println!(
                "Run workspace retained at: {:?}",
                workspace.workspace_path.parent()
            );
        }
        policy => {
            eprintln!(
                "Unknown cleanup policy: {}, defaulting to 'on_completion'",
                policy
            );
            remove_run_workspace(workspace)?;
        }
    }
    cleanup_old_instances(config)?;
    Ok(())
}

fn remove_run_workspace(workspace: &RunWorkspace) -> Result<()> {
    if let Some(run_dir) = workspace.workspace_path.parent() {
        if run_dir.exists() {
            std::fs::remove_dir_all(run_dir).context("Failed to remove run workspace")?;
            println!("Cleaned up run workspace: {:?}", run_dir);
        }
    }
    Ok(())
}

fn remove_workspace(workspace: &TaskWorkspace) -> Result<()> {
    if workspace.path.exists() {
        std::fs::remove_dir_all(&workspace.path).context("Failed to remove workspace")?;
        println!("Cleaned up workspace: {:?}", workspace.path);
    }
    Ok(())
}

fn cleanup_old_workspaces(config: &RuntimeConfig) -> Result<()> {
    let base_dir = &config.workspace.base_dir;
    if !base_dir.exists() {
        return Ok(());
    }

    let mut workspaces: Vec<_> = std::fs::read_dir(base_dir)?
        .filter_map(|entry| entry.ok())
        .filter(|entry| entry.file_name().to_string_lossy().starts_with("task-"))
        .filter_map(|entry| {
            let metadata = entry.metadata().ok()?;
            let modified = metadata.modified().ok()?;
            Some((entry.path(), modified))
        })
        .collect();

    workspaces.sort_by(|a, b| b.1.cmp(&a.1));

    let max_retained = config.workspace.max_retained as usize;
    for (path, _) in workspaces.iter().skip(max_retained) {
        if let Err(e) = std::fs::remove_dir_all(path) {
            eprintln!("Failed to cleanup old workspace {:?}: {}", path, e);
        } else {
            println!("Cleaned up old workspace: {:?}", path);
        }
    }

    Ok(())
}

fn cleanup_old_instances(config: &RuntimeConfig) -> Result<()> {
    let instances_dir = config.workspace.base_dir.join("instances");
    if !instances_dir.exists() {
        return Ok(());
    }

    let mut instances: Vec<_> = std::fs::read_dir(&instances_dir)?
        .filter_map(|entry| entry.ok())
        .filter_map(|entry| {
            let metadata = entry.metadata().ok()?;
            let modified = metadata.modified().ok()?;
            Some((entry.path(), modified))
        })
        .collect();

    instances.sort_by(|a, b| b.1.cmp(&a.1));

    let max_retained = config.workspace.max_retained as usize;
    for (path, _) in instances.iter().skip(max_retained) {
        if let Err(e) = std::fs::remove_dir_all(&path) {
            eprintln!("Failed to cleanup old instance {:?}: {}", path, e);
        } else {
            println!("Cleaned up old instance: {:?}", path);
        }
    }

    Ok(())
}

#[cfg(test)]
#[allow(deprecated)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn test_create_workspace() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();

        let workspace = create_task_workspace(&config, 123).unwrap();
        assert!(workspace.path.exists());
        assert_eq!(workspace.task_id, 123);
    }

    #[test]
    fn test_cleanup_on_completion() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();
        config.workspace.cleanup_policy = "on_completion".to_string();

        let workspace = create_task_workspace(&config, 456).unwrap();
        assert!(workspace.path.exists());

        cleanup_workspace(&workspace, &config).unwrap();
        assert!(!workspace.path.exists());
    }

    #[test]
    fn test_cleanup_never() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();
        config.workspace.cleanup_policy = "never".to_string();

        let workspace = create_task_workspace(&config, 789).unwrap();
        assert!(workspace.path.exists());

        cleanup_workspace(&workspace, &config).unwrap();
        assert!(workspace.path.exists());
    }

    #[test]
    fn test_create_run_workspace() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();

        let workspace = create_run_workspace(&config, "instance-1", "run-1").unwrap();
        assert!(workspace.workspace_path.exists());
        assert!(workspace.logs_path.exists());
        assert!(workspace.artifacts_path.exists());
    }

    #[test]
    fn test_run_workspace_isolation() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();

        let ws1 = create_run_workspace(&config, "instance-1", "run-1").unwrap();
        let ws2 = create_run_workspace(&config, "instance-1", "run-2").unwrap();

        assert_ne!(ws1.workspace_path, ws2.workspace_path);
        assert!(ws1.workspace_path.exists());
        assert!(ws2.workspace_path.exists());
    }
}
