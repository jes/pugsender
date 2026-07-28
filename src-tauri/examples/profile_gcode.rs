use pugsender_lib::gcode::build_toolpath;
use std::{env, fs, time::Instant};

fn main() -> Result<(), String> {
    let path = env::args()
        .nth(1)
        .ok_or_else(|| "usage: profile_gcode <path>".to_string())?;

    let read_started = Instant::now();
    let contents = fs::read_to_string(&path).map_err(|err| format!("read {path}: {err}"))?;
    let read_elapsed = read_started.elapsed();

    let split_started = Instant::now();
    let lines: Vec<String> = contents.lines().map(str::to_string).collect();
    let split_elapsed = split_started.elapsed();

    let build_started = Instant::now();
    let toolpath = build_toolpath(&lines);
    let build_elapsed = build_started.elapsed();

    println!("path: {path}");
    println!("bytes: {}", contents.len());
    println!("lines: {}", lines.len());
    println!("toolpath points: {}", toolpath.points.len());
    println!("estimated seconds: {:.1}", toolpath.estimated_seconds);
    println!("read: {:.3?}", read_elapsed);
    println!("split: {:.3?}", split_elapsed);
    println!("toolpath: {:.3?}", build_elapsed);
    println!(
        "total: {:.3?}",
        read_elapsed + split_elapsed + build_elapsed
    );

    Ok(())
}
