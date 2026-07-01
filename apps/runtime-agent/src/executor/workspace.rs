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

pub fn cleanup_run_workspace(workspace: &RunWorkspace, config: &RuntimeConfig) -> Result<()> {
    match config.workspace.cleanup_policy.as_str() {
        "on_success" | "on_completion" => {
            remove_run_workspace(workspace, config)?;
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
            remove_run_workspace(workspace, config)?;
        }
    }
    cleanup_old_instances(config)?;
    Ok(())
}

fn remove_run_workspace(workspace: &RunWorkspace, config: &RuntimeConfig) -> Result<()> {
    if !is_legacy_run_workspace(workspace, config) {
        println!(
            "Project task workspace retained at: {:?}",
            workspace.workspace_path
        );
        return Ok(());
    }

    if let Some(run_dir) = workspace.workspace_path.parent() {
        if run_dir.exists() {
            std::fs::remove_dir_all(run_dir).context("Failed to remove run workspace")?;
            println!("Cleaned up run workspace: {:?}", run_dir);
        }
    }
    Ok(())
}

fn is_legacy_run_workspace(workspace: &RunWorkspace, config: &RuntimeConfig) -> bool {
    workspace.workspace_path
        == config
            .workspace
            .base_dir
            .join("instances")
            .join(&workspace.execution_instance_id)
            .join("runs")
            .join(&workspace.run_id)
            .join("workspace")
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
mod tests {
    use super::*;
    use tempfile::TempDir;

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

    #[test]
    fn test_cleanup_retains_project_task_workspace() {
        let temp_dir = TempDir::new().unwrap();
        let mut config = RuntimeConfig::default();
        config.workspace.base_dir = temp_dir.path().to_path_buf();

        let workspace_path = config
            .workspace
            .base_dir
            .join("workspaces")
            .join("project-1")
            .join("task-1")
            .join("attempt-1");
        std::fs::create_dir_all(&workspace_path).unwrap();
        let workspace = RunWorkspace {
            workspace_path: workspace_path.clone(),
            logs_path: workspace_path.join("logs"),
            artifacts_path: workspace_path.join("artifacts"),
            execution_instance_id: "project-1".to_string(),
            run_id: "attempt-1".to_string(),
            created_at: Instant::now(),
        };

        cleanup_run_workspace(&workspace, &config).unwrap();

        assert!(workspace_path.exists());
    }
}
