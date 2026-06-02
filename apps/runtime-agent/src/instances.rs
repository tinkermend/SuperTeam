use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct EnsureInstanceRequest {
    pub base_dir: PathBuf,
    pub execution_instance_id: String,
}

#[derive(Debug, Clone)]
pub struct EnsureInstanceResult {
    pub agent_home_dir: PathBuf,
}

pub fn ensure_instance(request: EnsureInstanceRequest) -> anyhow::Result<EnsureInstanceResult> {
    let agent_home_dir = request
        .base_dir
        .join("agents")
        .join(sanitize_segment(&request.execution_instance_id)?);
    std::fs::create_dir_all(agent_home_dir.join("state"))?;
    std::fs::create_dir_all(agent_home_dir.join("sessions"))?;
    std::fs::create_dir_all(agent_home_dir.join("runs"))?;
    Ok(EnsureInstanceResult { agent_home_dir })
}

fn sanitize_segment(value: &str) -> anyhow::Result<String> {
    if value.is_empty()
        || value.contains('/')
        || value.contains('\\')
        || value == "."
        || value == ".."
    {
        anyhow::bail!("invalid execution instance id");
    }
    Ok(value.to_string())
}
