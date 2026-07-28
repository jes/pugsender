# pugsender

Keyboard-first G-code sender for Grbl/grblHAL controllers.

This is a Rust/Tauri/Svelte rewrite. The previous Go/Gio prototype has been
kept under `legacy/` for reference.

## Development

On Ubuntu 24.04, install the native Tauri/WebKit build dependencies first:

```sh
sudo apt update
sudo apt install -y \
  build-essential curl file libssl-dev pkg-config \
  libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev \
  libwebkit2gtk-4.1-dev
```

Install Rust with rustup if `cargo` is not already available:

```sh
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
. "$HOME/.cargo/env"
```

Install Node.js 20 LTS or newer. If `node --version` is older, install it
through your preferred Node version manager or the NodeSource package.

Then install the JavaScript dependencies and start the desktop app:

```sh
npm install
npm run tauri dev
```

If you want to connect to a real controller later, add your user to the serial
port group and log out/in before using `/dev/ttyUSB*` or `/dev/ttyACM*`:

```sh
sudo usermod -aG dialout "$USER"
```

Useful checks:

```sh
npm run check
npm run build
npm run tauri build
cargo test --manifest-path src-tauri/Cargo.toml --no-default-features
cargo test --manifest-path src-tauri/Cargo.toml --no-default-features --features serial
cargo test --manifest-path src-tauri/Cargo.toml --features desktop-test
cargo clippy --manifest-path src-tauri/Cargo.toml --no-default-features -- -D warnings
cargo clippy --manifest-path src-tauri/Cargo.toml --no-default-features --features serial -- -D warnings
cargo clippy --manifest-path src-tauri/Cargo.toml --features desktop-test -- -D warnings
```

The no-default Rust test suite includes a burst-load streaming test with a tiny
simulated controller RX buffer. It verifies that Pugsender only transmits a line
when the full normalized line plus newline fits in the controller's reported
free serial space.

To profile G-code loading and toolpath generation without the UI:

```sh
cargo run --manifest-path src-tauri/Cargo.toml --no-default-features --example profile_gcode -- /path/to/file.nc
```

Optional physical-controller smoke test:

```sh
PUGSENDER_SERIAL_PORT=/dev/ttyUSB0 cargo test --manifest-path src-tauri/Cargo.toml --features serial --test serial_pty machine_streams_program_through_physical_grbl_controller -- --ignored
```

Optional physical auto-connect test:

```sh
PUGSENDER_SERIAL_PORT=/dev/ttyUSB0 cargo test --manifest-path src-tauri/Cargo.toml --features desktop-test auto_connect_finds_physical_grbl_controller -- --ignored
```
