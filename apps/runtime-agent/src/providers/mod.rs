pub mod claude;
pub mod opencode;

use std::path::PathBuf;
use std::process::ExitStatus;
use std::sync::Arc;

use async_trait::async_trait;
use futures::StreamExt;
use futures::stream::{self, BoxStream};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex;
use tokio::task::JoinHandle;

use crate::events::ProviderEvent;

pub type ProviderEventStream = BoxStream<'static, anyhow::Result<ProviderEvent>>;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProviderRequest {
    pub prompt: String,
    pub workspace_path: PathBuf,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
}

#[async_trait]
pub trait ProviderAdapter: Send + Sync {
    async fn start(&self, request: ProviderRequest) -> anyhow::Result<ProviderRun>;

    async fn run(&self, request: ProviderRequest) -> anyhow::Result<ProviderEventStream> {
        Ok(self.start(request).await?.events)
    }
}

type ProviderParser = fn(&str) -> anyhow::Result<Option<ProviderEvent>>;

struct ChildStreamState {
    provider_name: &'static str,
    parser: ProviderParser,
    lines: tokio::io::Lines<BufReader<ChildStdout>>,
    child: SharedChild,
    stderr_task: JoinHandle<std::io::Result<String>>,
}

type SharedChild = Arc<Mutex<Child>>;

#[derive(Clone)]
pub struct ProviderRunHandle {
    child: SharedChild,
}

impl ProviderRunHandle {
    pub async fn cancel(&self) -> anyhow::Result<()> {
        let mut child = self.child.lock().await;
        child
            .kill()
            .await
            .map_err(|error| anyhow::anyhow!("failed to cancel provider process: {error}"))
    }
}

pub struct ProviderRun {
    pub events: ProviderEventStream,
    pub handle: ProviderRunHandle,
}

pub fn stream_child_events(
    provider_name: &'static str,
    parser: ProviderParser,
    child: Child,
    stdout: ChildStdout,
    stderr: ChildStderr,
) -> ProviderRun {
    let child = Arc::new(Mutex::new(child));
    let handle = ProviderRunHandle {
        child: child.clone(),
    };
    let stderr_task = tokio::spawn(async move {
        let mut stderr_text = String::new();
        let mut reader = BufReader::new(stderr);
        reader.read_to_string(&mut stderr_text).await?;
        Ok(stderr_text)
    });

    let state = ChildStreamState {
        provider_name,
        parser,
        lines: BufReader::new(stdout).lines(),
        child: child.clone(),
        stderr_task,
    };

    let events = stream::unfold(Some(state), |state| async move {
        let mut state = state?;

        loop {
            match state.lines.next_line().await {
                Ok(Some(line)) => match (state.parser)(&line) {
                    Ok(Some(event)) => return Some((Ok(event), Some(state))),
                    Ok(None) => continue,
                    Err(error) => return Some((Err(error), Some(state))),
                },
                Ok(None) => {
                    let status = state.child.lock().await.wait().await;
                    let stderr = read_stderr(state.stderr_task).await;
                    return provider_exit_result(state.provider_name, status, stderr)
                        .map(|result| (result, None));
                }
                Err(error) => return Some((Err(error.into()), None)),
            }
        }
    })
    .boxed();

    ProviderRun { events, handle }
}

async fn read_stderr(stderr_task: JoinHandle<std::io::Result<String>>) -> String {
    match stderr_task.await {
        Ok(Ok(stderr)) => stderr.trim().to_string(),
        Ok(Err(error)) => format!("failed to read stderr: {error}"),
        Err(error) => format!("failed to join stderr reader: {error}"),
    }
}

fn provider_exit_result(
    provider_name: &str,
    status: std::io::Result<ExitStatus>,
    stderr: String,
) -> Option<anyhow::Result<ProviderEvent>> {
    match status {
        Ok(status) if status.success() => None,
        Ok(status) => {
            let code = status
                .code()
                .map(|code| code.to_string())
                .unwrap_or_else(|| "signal".to_string());
            let mut message = format!("{provider_name} exited with status {code}");
            if !stderr.is_empty() {
                message.push_str(": ");
                message.push_str(&stderr);
            }
            Some(Err(anyhow::anyhow!(message)))
        }
        Err(error) => Some(Err(anyhow::anyhow!(
            "failed to wait for {provider_name}: {error}"
        ))),
    }
}
