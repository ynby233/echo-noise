#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::{thread, time::Duration, fs};
use std::env;
use std::process::Command as SysCommand;
use tauri::{Manager};

fn wait_ready(url: &str, timeout_ms: u64) -> bool {
  let start = std::time::Instant::now();
  while start.elapsed().as_millis() < timeout_ms as u128 {
    // try /api/status then /status
    for path in ["/api/status", "/status"].iter() {
      if let Ok(resp) = reqwest::blocking::get(format!("{}{}", url, path)) {
        if resp.status().is_success() { return true; }
      }
    }
    thread::sleep(Duration::from_millis(500));
  }
  false
}

#[tauri::command]
fn start_server(app: tauri::AppHandle) {
  let resolver = app.path();
  let data_dir = app.path().app_data_dir().unwrap_or(app.path().resource_dir().unwrap());
  let db_path = data_dir.join("noise.db");
  let res_db = app.path().resource_dir().map(|p| p.join("data").join("noise.db")).unwrap();
  if !db_path.exists() {
    let _ = fs::create_dir_all(&data_dir);
    if res_db.exists() {
      let _ = fs::copy(&res_db, &db_path);
    } else {
      let _ = fs::File::create(&db_path);
    }
  }
  let _ = fs::create_dir_all(&data_dir);

  let res_dir = app.path().resource_dir().unwrap();
  let name = if cfg!(windows) { "server.exe" } else { "server" };
  let candidates = [
    res_dir.join("bin").join(name),
    res_dir.parent().unwrap_or(&res_dir).join("MacOS").join(name),
  ];
  let server_path = candidates.iter().find(|p| p.exists()).cloned().unwrap_or(res_dir.join("bin").join(name));
  let ver = app.package_info().version.to_string();
  let tag = format!("V{}", ver);
  let _ = SysCommand::new(server_path)
    .env("DB_TYPE", "sqlite")
    .env("DB_PATH", db_path.to_string_lossy().to_string())
    .env("APP_VERSION", ver.clone())
    .env("ECHO_NOISE_VERSION", ver.clone())
    .env("IMAGE_TAG", tag.clone())
    .current_dir(res_dir)
    .spawn();
}

fn main() {
  tauri::Builder::default()
    .setup(|app| {
      start_server(app.handle().clone());
      let url = "http://127.0.0.1:1314";
      let win = app.get_webview_window("main").unwrap_or_else(|| tauri::WebviewWindowBuilder::new(app, "main", tauri::WebviewUrl::App("index.html".into())).build().unwrap());
      if wait_ready(url, 20000) {
        let _ = win.navigate(url.parse().unwrap());
      }
      Ok(())
    })
    .invoke_handler(tauri::generate_handler![start_server])
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}
