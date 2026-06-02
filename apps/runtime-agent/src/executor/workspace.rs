use anyhow::{Context, Result};
use std::path::PathBuf;
use std::time::Instant;

use crate::config::RuntimeConfig;

pub struct TaskWorkspace {
    pub path: PathBuf,
    pub task_id: i64,
    pub created_at: Instant,
}

pub fn create_task_workspace(config: &RuntimeConfig, task_id: i64) -> Result<TaskWorkspace> {
    let workspace_path = config.workspace.base_dir.join(format!("task-{}", task_id));

    std::fs::create_dir_all(&workspace_path).context("Failed to create task workspace")?;

    Ok(TaskWorkspace {
        path: workspace_path,
        task_id,
        created_at: Instant::now(),
    })
}

pub fn cleanup_workspace(workspace: &TaskWorkspace, config: &RuntimeConfig) -> Result<()> {
    match config.workspace.cleanup_policy.as_str() {
        "on_success" => {
            remove_workspace(workspace)?;
        }
        "on_completion" => {
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

#[cfg(test)]
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
}
