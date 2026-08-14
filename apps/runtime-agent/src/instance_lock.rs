use std::fs::{File, OpenOptions};
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};

const LOCK_DIR_ENV: &str = "RUNTIME_AGENT_INSTANCE_LOCK_DIR";

/// Held for the lifetime of a running daemon so the OS releases the flock on exit.
#[derive(Debug)]
pub struct NodeInstanceLock {
    pub path: PathBuf,
    _file: File,
}

pub fn acquire_node_instance_lock(node_id: &str) -> Result<NodeInstanceLock> {
    acquire_node_instance_lock_in(node_id, &default_lock_dir()?)
}

pub fn acquire_node_instance_lock_in(node_id: &str, lock_dir: &Path) -> Result<NodeInstanceLock> {
    let node_id = node_id.trim();
    anyhow::ensure!(!node_id.is_empty(), "node id is required");
    std::fs::create_dir_all(lock_dir).with_context(|| {
        format!(
            "create runtime-agent instance lock dir {}",
            lock_dir.display()
        )
    })?;
    let path = lock_dir.join(format!("{}.lock", sanitize_node_id(node_id)));
    let mut file = OpenOptions::new()
        .read(true)
        .write(true)
        .create(true)
        .truncate(false)
        .open(&path)
        .with_context(|| format!("open runtime-agent instance lock {}", path.display()))?;
    if !try_lock_exclusive(&file)? {
        let holder = read_holder_pid(&mut file);
        let pid_note = holder
            .map(|pid| format!("pid {pid}"))
            .unwrap_or_else(|| "unknown pid".to_string());
        anyhow::bail!(
            "runtime-agent already running for node `{node_id}` ({pid_note}; lock {}); this host allows only one instance per node",
            path.display()
        );
    }
    file.set_len(0)?;
    file.seek(SeekFrom::Start(0))?;
    write!(file, "{}", std::process::id())?;
    file.flush()?;
    Ok(NodeInstanceLock { path, _file: file })
}

pub fn default_lock_dir() -> Result<PathBuf> {
    if let Some(dir) = std::env::var_os(LOCK_DIR_ENV) {
        return Ok(PathBuf::from(dir));
    }
    let home =
        std::env::var("HOME").context("HOME is required for the default instance lock dir")?;
    Ok(PathBuf::from(home)
        .join(".superteam")
        .join("runtime-agent")
        .join("locks"))
}

fn sanitize_node_id(node_id: &str) -> String {
    let mut out = String::with_capacity(node_id.len());
    for ch in node_id.chars() {
        if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' || ch == '.' {
            out.push(ch);
        } else {
            out.push('_');
        }
    }
    if out.len() > 128 {
        out.truncate(128);
    }
    if out.is_empty() {
        "node".to_string()
    } else {
        out
    }
}

fn read_holder_pid(file: &mut File) -> Option<u32> {
    let mut buf = String::new();
    file.seek(SeekFrom::Start(0)).ok()?;
    file.read_to_string(&mut buf).ok()?;
    buf.trim().parse().ok()
}

#[cfg(unix)]
fn try_lock_exclusive(file: &File) -> Result<bool> {
    use std::os::unix::io::AsRawFd;
    let rc = unsafe { libc::flock(file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) };
    if rc == 0 {
        return Ok(true);
    }
    let err = std::io::Error::last_os_error();
    if err.kind() == std::io::ErrorKind::WouldBlock
        || err.raw_os_error() == Some(libc::EWOULDBLOCK)
        || err.raw_os_error() == Some(libc::EAGAIN)
    {
        return Ok(false);
    }
    Err(err).context("flock exclusive")
}

#[cfg(not(unix))]
fn try_lock_exclusive(_file: &File) -> Result<bool> {
    anyhow::bail!("runtime-agent instance lock requires unix flock")
}

#[cfg(test)]
mod tests {
    use super::{acquire_node_instance_lock_in, sanitize_node_id};

    #[test]
    fn sanitize_replaces_unsafe_node_id_chars() {
        assert_eq!(sanitize_node_id("local-dev-node"), "local-dev-node");
        assert_eq!(sanitize_node_id("a/b:c"), "a_b_c");
    }

    #[test]
    fn same_process_can_release_and_reacquire() {
        let dir = tempfile::TempDir::new().expect("tempdir");
        let first = acquire_node_instance_lock_in("mutex-node", dir.path()).expect("first lock");
        drop(first);
        acquire_node_instance_lock_in("mutex-node", dir.path()).expect("reacquire after drop");
    }
}
