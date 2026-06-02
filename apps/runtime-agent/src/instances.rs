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
    if !is_uuid_like(value) {
        anyhow::bail!("invalid execution instance id");
    }
    Ok(value.to_string())
}

fn is_uuid_like(value: &str) -> bool {
    if value.len() != 36 {
        return false;
    }
    for (index, ch) in value.chars().enumerate() {
        match index {
            8 | 13 | 18 | 23 => {
                if ch != '-' {
                    return false;
                }
            }
            _ => {
                if !ch.is_ascii_hexdigit() {
                    return false;
                }
            }
        }
    }
    true
}
