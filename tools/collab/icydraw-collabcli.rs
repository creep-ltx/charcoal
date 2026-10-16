// IcyDraw's collaboration client (icy_engine_edit::collaboration::connect) for
// Charcoal's tests: collabcli <ws url> <password>. Joins, draws a cell at the
// page's last cell, chats, waits for a cell at 0,0 with code 77 ("M"), then
// prints the page it ends with as JSON (row-major {code, fg, bg}).
use icy_engine_edit::collaboration::{connect, Block, ClientConfig, CollaborationEvent};

fn main() {
    let a: Vec<String> = std::env::args().collect();
    let rt = tokio::runtime::Builder::new_multi_thread().enable_all().build().unwrap();
    rt.block_on(async {
        let cfg = ClientConfig { url: a[1].clone(), nick: "icydraw".into(), group: "test".into(), password: a[2].clone(), ping_interval_secs: 30 };
        let (h, mut ev) = connect(cfg).await.expect("connect");
        let (mut cols, mut rows) = (0u32, 0u32);
        let mut cells: Vec<Block> = Vec::new();
        let mut title = String::new();
        let mut chat = Vec::new();
        let deadline = tokio::time::sleep(std::time::Duration::from_secs(15));
        tokio::pin!(deadline);
        loop {
            tokio::select! {
                _ = &mut deadline => { eprintln!("timed out"); break; }
                e = ev.recv() => {
                    let Some(e) = e else { break };
                    match e {
                        CollaborationEvent::Connected(d) => {
                            cols = d.columns; rows = d.rows; title = d.title.clone();
                            cells = vec![Block::default(); (cols * rows) as usize];
                            for (x, col) in d.document.iter().enumerate() {
                                for (y, b) in col.iter().enumerate() { cells[y * cols as usize + x] = b.clone(); }
                            }
                            let b = Block { code: 219, fg: 4, bg: 1 };
                            cells[((rows - 1) * cols + cols - 1) as usize] = b.clone();
                            h.draw(cols as i32 - 1, rows as i32 - 1, b).await.unwrap();
                            h.send_chat("hello from IcyDraw".into()).await.unwrap();
                        }
                        CollaborationEvent::Draw { col, row, block } => {
                            if (col as u32) < cols && (row as u32) < rows { cells[(row as u32 * cols + col as u32) as usize] = block.clone(); }
                            if col == 0 && row == 0 && block.code == 77 { break; }
                        }
                        CollaborationEvent::Chat(c) => chat.push(c.text),
                        CollaborationEvent::Refused { reason } => { eprintln!("refused: {reason}"); break; }
                        CollaborationEvent::Disconnected => break,
                        _ => {}
                    }
                }
            }
        }
        let data: Vec<String> = cells.iter().map(|b| format!("{{\"code\":{},\"fg\":{},\"bg\":{}}}", b.code, b.fg, b.bg)).collect();
        println!("{{\"columns\":{},\"rows\":{},\"title\":{},\"chat\":{},\"data\":[{}]}}", cols, rows,
            serde_json::to_string(&title).unwrap(), serde_json::to_string(&chat).unwrap(), data.join(","));
        let _ = h.disconnect().await;
    });
}
