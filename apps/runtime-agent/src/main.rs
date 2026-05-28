use clap::Parser;
use superteam_runtime_agent::config::RuntimeConfig;
use superteam_runtime_agent::daemon::RuntimeDaemon;

#[derive(Debug, Parser)]
struct Args {
    #[arg(long, env = "RUNTIME_NODE_ID", default_value = "local-dev-node")]
    node_id: String,

    #[arg(long)]
    once: bool,
}

fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    let daemon = RuntimeDaemon::new(RuntimeConfig::new(args.node_id)?);
    let snapshot = daemon.snapshot();
    println!(
        "runtime-agent node={} status={}",
        snapshot.node_id, snapshot.status
    );
    if args.once {
        return Ok(());
    }
    loop {
        std::thread::park();
    }
}
