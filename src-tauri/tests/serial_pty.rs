#![cfg(feature = "serial")]

use nix::{
    fcntl::OFlag,
    pty::{grantpt, posix_openpt, ptsname, unlockpt},
};
use pugsender_lib::{machine::Machine, serial_link::SerialLink};
use std::{
    env,
    fs::File,
    io::{Read, Write},
    os::fd::{FromRawFd, IntoRawFd},
    sync::{mpsc, Arc, Mutex},
    thread,
    time::{Duration, Instant},
};

#[test]
fn serial_link_round_trips_lines_through_a_pty() {
    let master = posix_openpt(OFlag::O_RDWR | OFlag::O_NOCTTY).expect("open pty");
    grantpt(&master).expect("grant pty");
    unlockpt(&master).expect("unlock pty");
    let slave_name = unsafe { ptsname(&master).expect("slave path") };
    let mut master_file = unsafe { File::from_raw_fd(master.into_raw_fd()) };

    let (rx_tx, rx) = mpsc::channel();
    let link = SerialLink::open(&slave_name, 115_200, move |line| {
        rx_tx.send(line).expect("send received line");
    })
    .expect("open serial link");

    link.send("G0 X1\n".to_string()).expect("send command");

    let mut bytes = [0_u8; 64];
    let read = master_file.read(&mut bytes).expect("read master");
    assert_eq!(&bytes[..read], b"G0 X1\n");

    master_file.write_all(b"ok\n").expect("write response");
    let line = rx
        .recv_timeout(Duration::from_secs(2))
        .expect("receive line");
    assert_eq!(line, "ok");
}

#[test]
fn machine_streams_program_through_mock_serial_controller() {
    let master = posix_openpt(OFlag::O_RDWR | OFlag::O_NOCTTY).expect("open pty");
    grantpt(&master).expect("grant pty");
    unlockpt(&master).expect("unlock pty");
    let slave_name = unsafe { ptsname(&master).expect("slave path") };
    let mut master_file = unsafe { File::from_raw_fd(master.into_raw_fd()) };
    let observed = Arc::new(Mutex::new(Vec::new()));
    let mock_observed = Arc::clone(&observed);

    let mock = thread::spawn(move || {
        let mut buf = [0_u8; 1];
        let mut pending = Vec::new();
        loop {
            let Ok(read) = master_file.read(&mut buf) else {
                break;
            };
            if read == 0 {
                break;
            }
            if pending.is_empty() && matches!(buf[0], b'~' | b'!' | 0x18) {
                mock_observed
                    .lock()
                    .expect("observed lock")
                    .push(String::from_utf8_lossy(&buf[..1]).to_string());
                continue;
            }
            if buf[0] != b'\n' {
                pending.push(buf[0]);
                continue;
            }

            let line = String::from_utf8_lossy(&pending)
                .trim_end_matches('\r')
                .to_string();
            pending.clear();
            mock_observed
                .lock()
                .expect("observed lock")
                .push(line.clone());

            if line == "$I+" {
                master_file
                    .write_all(
                        b"[FIRMWARE:grblHAL]\n[AXS:4:XYZA]\n<Idle|MPos:0.000,0.000,0.000,0.000|Bf:15,128>\nok\n",
                    )
                    .expect("write startup response");
            } else if line == "$I" {
                master_file.write_all(b"ok\n").expect("write info ok");
            } else if line.starts_with('G') {
                master_file.write_all(b"ok\n").expect("write ok");
            }
        }
    });

    let machine = Arc::new(Mutex::new(Machine::new()));
    let callback_machine = Arc::clone(&machine);
    let link = SerialLink::open(&slave_name, 115_200, move |line| {
        callback_machine
            .lock()
            .expect("machine lock")
            .receive_controller_line(&line);
    })
    .expect("open serial link");

    machine
        .lock()
        .expect("machine lock")
        .connect_port(slave_name.clone());
    {
        let mut machine = machine.lock().expect("machine lock");
        machine.queue_startup_probe();
        link.send_all(machine.take_outbox())
            .expect("send startup probe");
    }
    wait_until(Duration::from_secs(2), || {
        machine
            .lock()
            .expect("machine lock")
            .snapshot()
            .status
            .ready
    });

    {
        let mut machine = machine.lock().expect("machine lock");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();
        machine.run_program().expect("run program");
        link.send_all(machine.take_outbox())
            .expect("send program outbox");
    }

    wait_until(Duration::from_secs(2), || {
        let mut machine = machine.lock().expect("machine lock");
        link.send_all(machine.take_outbox()).expect("flush outbox");
        !machine.snapshot().program.running && machine.snapshot().program.current_line.is_none()
    });

    drop(link);
    mock.join().expect("mock controller thread");

    let snapshot = machine.lock().expect("machine lock").snapshot();
    assert_eq!(
        snapshot.status.capabilities.firmware.as_deref(),
        Some("grblHAL")
    );
    assert_eq!(snapshot.status.axis_count, 4);
    assert_eq!(snapshot.status.capabilities.axis_letters, "XYZA");
    assert_eq!(snapshot.program.next_line, 2);
    assert!(snapshot.program.queued_lines.is_empty());

    let observed = observed.lock().expect("observed lock");
    assert!(observed.iter().any(|line| line == "$I+"));
    assert!(observed.iter().any(|line| line == "G0 X1"));
    assert!(observed.iter().any(|line| line == "G0 X2"));
}

#[test]
#[ignore = "requires a physical Grbl-compatible controller; set PUGSENDER_SERIAL_PORT"]
fn machine_streams_program_through_physical_grbl_controller() {
    let port = env::var("PUGSENDER_SERIAL_PORT")
        .expect("set PUGSENDER_SERIAL_PORT to a Grbl serial device, for example /dev/ttyUSB0");
    let machine = Arc::new(Mutex::new(Machine::new()));
    let callback_machine = Arc::clone(&machine);
    let link = SerialLink::open(&port, 115_200, move |line| {
        callback_machine
            .lock()
            .expect("machine lock")
            .receive_controller_line(&line);
    })
    .expect("open physical serial link");

    {
        let mut machine = machine.lock().expect("machine lock");
        machine.connect_port(port.clone());
        machine.queue_startup_probe();
        link.send_all(machine.take_outbox())
            .expect("send startup probe");
    }
    link.poll_every("?".to_string(), Duration::from_millis(200));

    wait_until(Duration::from_secs(5), || {
        let snapshot = machine.lock().expect("machine lock").snapshot();
        snapshot.status.ready
            && matches!(
                snapshot.status.mode,
                pugsender_lib::machine::MachineMode::Idle
            )
    });

    {
        let mut machine = machine.lock().expect("machine lock");
        machine
            .load_gcode_text(
                "hardware-smoke.nc".to_string(),
                "G90\nG0 X1\nG1 X2 F120\nG4 P0.1\nG0 X0\nM30\nG0 X99\n".to_string(),
            )
            .unwrap();
        machine.run_program().expect("run program");
        link.send_all(machine.take_outbox())
            .expect("send initial program outbox");
    }

    wait_until(Duration::from_secs(10), || {
        let mut machine = machine.lock().expect("machine lock");
        link.send_all(machine.take_outbox()).expect("flush outbox");
        let snapshot = machine.snapshot();
        !snapshot.program.running
            && snapshot.program.current_line.is_none()
            && snapshot.program.next_line == snapshot.program.lines.len()
    });

    let snapshot = machine.lock().expect("machine lock").snapshot();
    assert!(snapshot.status.ready);
    assert!(snapshot.program.queued_lines.is_empty());
    assert_eq!(
        snapshot.status.capabilities.version.as_deref(),
        Some("1.1h")
    );
}

fn wait_until(timeout: Duration, mut condition: impl FnMut() -> bool) {
    let started = Instant::now();
    while started.elapsed() < timeout {
        if condition() {
            return;
        }
        thread::sleep(Duration::from_millis(10));
    }
    assert!(condition(), "condition was not met within {timeout:?}");
}
