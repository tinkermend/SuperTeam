use std::io::Write;
use std::net::SocketAddr;
use std::path::PathBuf;

use clap::Parser;
use clap::Subcommand;
use clap::ValueEnum;
use futures::StreamExt;
use superteam_runtime_agent::config::{RuntimeConfig, RuntimeConfigOverrides};
use superteam_runtime_agent::daemon::RuntimeDaemon;
use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::opencode::OpenCodeProvider;
use superteam_runtime_agent::providers::{ProviderAdapter, ProviderRequest};

#[derive(Debug, Parser)]
struct Args {
    #[arg(long)]
    config: Option<PathBuf>,

    #[arg(long)]
    node_id: Option<String>,

    #[arg(long)]
    bootstrap_key: Option<String>,

    #[arg(long)]
    auth_token: Option<String>,

    #[arg(long)]
    once: bool,

    #[arg(long)]
    http_addr: Option<SocketAddr>,

    #[arg(long)]
    run_log_dir: Option<PathBuf>,

    #[arg(long)]
    claude_bin: Option<PathBuf>,

    #[arg(long)]
    opencode_bin: Option<PathBuf>,

    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Debug, Subcommand)]
enum Commands {
    Run(RunArgs),
}

#[derive(Debug, Parser)]
struct RunArgs {
    #[arg(long, value_enum)]
    provider: ProviderKind,

    #[arg(long)]
    provider_bin: Option<PathBuf>,

    #[arg(long)]
    workspace: PathBuf,

    #[arg(long)]
    prompt: String,

    #[arg(long)]
    session_id: Option<String>,

    #[arg(long)]
    continue_session: bool,

    #[arg(long)]
    model: Option<String>,
}

#[derive(Debug, Clone, Copy, ValueEnum)]
enum ProviderKind {
    Claude,
    Opencode,
}

impl ProviderKind {
    fn default_bin(self) -> &'static str {
        match self {
            Self::Claude => "claude",
            Self::Opencode => "opencode",
        }
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    if let Some(command) = args.command {
        return match command {
            Commands::Run(run_args) => run_provider(run_args).await,
        };
    }

    let config = RuntimeConfig::load(
        args.config.as_deref(),
        RuntimeConfigOverrides {
            node_id: args.node_id,
            bootstrap_key: args.bootstrap_key,
            auth_token: args.auth_token,
            http_addr: args.http_addr,
            run_log_dir: args.run_log_dir,
            claude_bin: args.claude_bin,
            opencode_bin: args.opencode_bin,
        },
    )?;
    let daemon = RuntimeDaemon::new(config);
    let snapshot = daemon.snapshot();
    println!(
        "runtime-agent node={} status={}",
        snapshot.node_id, snapshot.status
    );
    if args.once {
        return Ok(());
    }
    daemon.run().await
}

async fn run_provider(args: RunArgs) -> anyhow::Result<()> {
    let request = ProviderRequest {
        prompt: args.prompt,
        workspace_path: args.workspace,
        session_id: args.session_id,
        continue_session: args.continue_session,
        model: args.model,
    };
    let provider_bin = args
        .provider_bin
        .unwrap_or_else(|| PathBuf::from(args.provider.default_bin()));

    match args.provider {
        ProviderKind::Claude => {
            let provider = ClaudeProvider::new(provider_bin);
            stream_provider_events(&provider, request).await
        }
        ProviderKind::Opencode => {
            let provider = OpenCodeProvider::new(provider_bin);
            stream_provider_events(&provider, request).await
        }
    }
}

async fn stream_provider_events(
    provider: &impl ProviderAdapter,
    request: ProviderRequest,
) -> anyhow::Result<()> {
    let mut stream = match provider.run(request).await {
        Ok(stream) => stream,
        Err(error) => {
            emit_event(&ProviderEvent::TurnError {
                message: error.to_string(),
            })?;
            return Err(error);
        }
    };

    while let Some(event) = stream.next().await {
        match event {
            Ok(event) => emit_event(&event)?,
            Err(error) => {
                emit_event(&ProviderEvent::TurnError {
                    message: error.to_string(),
                })?;
                return Err(error);
            }
        }
    }

    Ok(())
}

fn emit_event(event: &ProviderEvent) -> anyhow::Result<()> {
    let mut stdout = std::io::stdout().lock();
    serde_json::to_writer(&mut stdout, event)?;
    stdout.write_all(b"\n")?;
    stdout.flush()?;
    Ok(())
}
