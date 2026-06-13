use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct EnsureInstanceRequest {
    pub base_dir: PathBuf,
    pub team_id: String,
    pub digital_employee_id: String,
}

#[derive(Debug, Clone)]
pub struct EnsureInstanceResult {
    pub agent_home_dir: PathBuf,
}

pub fn ensure_instance(request: EnsureInstanceRequest) -> anyhow::Result<EnsureInstanceResult> {
    let agent_home_dir = request
        .base_dir
        .join("teams")
        .join(sanitize_segment("team_id", &request.team_id)?)
        .join("employees")
        .join(sanitize_segment(
            "digital_employee_id",
            &request.digital_employee_id,
        )?);
    std::fs::create_dir_all(&agent_home_dir)?;
    Ok(EnsureInstanceResult { agent_home_dir })
}

fn sanitize_segment(field: &str, value: &str) -> anyhow::Result<String> {
    if !is_uuid_like(value) {
        anyhow::bail!("{field} must be a UUID-like string");
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
