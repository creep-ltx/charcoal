// A headless IcyDraw collaboration server for Charcoal's tests:
// collabsrv <port> <password> <columns> <rows> <backup folder>
use icy_engine_edit::collaboration::{run_server, ServerConfig};

fn main() {
    let a: Vec<String> = std::env::args().collect();
    let mut c = ServerConfig::default();
    c.bind_addr = format!("127.0.0.1:{}", a[1]).parse().unwrap();
    c.password = a[2].clone();
    c.columns = a[3].parse().unwrap();
    c.rows = a[4].parse().unwrap();
    c.autosave.backup_folder = a[5].clone().into();
    c.autosave.interval = None;
    let rt = tokio::runtime::Builder::new_multi_thread().enable_all().build().unwrap();
    if let Err(e) = rt.block_on(run_server(c)) {
        eprintln!("collabsrv: {e}");
        std::process::exit(1);
    }
}
