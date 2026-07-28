#[cfg(feature = "serial")]
use std::{
    collections::VecDeque,
    io::{BufRead, BufReader, Write},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Condvar, Mutex,
    },
    thread,
    time::Duration,
};

#[cfg(feature = "serial")]
type CommandQueue = Arc<(Mutex<VecDeque<String>>, Condvar)>;

#[cfg(feature = "serial")]
pub struct SerialLink {
    queue: CommandQueue,
    stop: Arc<AtomicBool>,
}

#[cfg(feature = "serial")]
impl SerialLink {
    pub fn open(
        port_name: &str,
        baud_rate: u32,
        mut on_line: impl FnMut(String) + Send + 'static,
    ) -> Result<Self, String> {
        let mut port = serialport::new(port_name, baud_rate)
            .timeout(Duration::from_millis(100))
            .open()
            .map_err(|err| format!("open {port_name}: {err}"))?;
        let reader_port = port
            .try_clone()
            .map_err(|err| format!("clone {port_name}: {err}"))?;
        let queue = Arc::new((Mutex::new(VecDeque::<String>::new()), Condvar::new()));
        let stop = Arc::new(AtomicBool::new(false));
        let reader_stop = Arc::clone(&stop);
        let writer_stop = Arc::clone(&stop);
        let writer_queue = Arc::clone(&queue);

        thread::spawn(move || {
            let mut reader = BufReader::new(reader_port);
            let mut line = String::new();
            while !reader_stop.load(Ordering::Relaxed) {
                line.clear();
                match reader.read_line(&mut line) {
                    Ok(0) => {}
                    Ok(_) => on_line(line.trim_end_matches(['\r', '\n']).to_string()),
                    Err(err) if err.kind() == std::io::ErrorKind::TimedOut => {}
                    Err(_) => break,
                }
            }
        });

        thread::spawn(move || {
            while !writer_stop.load(Ordering::Relaxed) {
                let command = {
                    let (lock, condvar) = &*writer_queue;
                    let mut commands = lock.lock().expect("serial queue mutex poisoned");
                    loop {
                        if writer_stop.load(Ordering::Relaxed) {
                            return;
                        }
                        if let Some(command) = commands.pop_front() {
                            break command;
                        }
                        let (next, _) = condvar
                            .wait_timeout(commands, Duration::from_millis(100))
                            .expect("serial queue mutex poisoned");
                        commands = next;
                    }
                };
                let bytes = command_bytes(&command);
                if port.write_all(&bytes).is_err() {
                    break;
                }
                if port.flush().is_err() {
                    break;
                }
            }
        });

        Ok(Self { queue, stop })
    }

    pub fn send(&self, command: String) -> Result<(), String> {
        enqueue_command(&self.queue, &self.stop, command)
    }

    pub fn send_all(&self, commands: Vec<String>) -> Result<(), String> {
        if self.stop.load(Ordering::Relaxed) {
            return Err("serial writer is closed".to_string());
        }
        if commands.is_empty() {
            return Ok(());
        }
        let (lock, condvar) = &*self.queue;
        let mut queue = lock.lock().expect("serial queue mutex poisoned");
        for command in commands {
            if is_realtime_command(&command) {
                let index = queue
                    .iter()
                    .take_while(|queued| is_realtime_command(queued))
                    .count();
                queue.insert(index, command);
            } else {
                queue.push_back(command);
            }
        }
        condvar.notify_all();
        Ok(())
    }

    pub fn clear_pending(&self) {
        let (lock, _) = &*self.queue;
        lock.lock().expect("serial queue mutex poisoned").clear();
    }

    pub fn poll_every(&self, command: String, interval: Duration) {
        let queue = Arc::clone(&self.queue);
        let stop = Arc::clone(&self.stop);
        thread::spawn(move || {
            while !stop.load(Ordering::Relaxed) {
                let _ = enqueue_command(&queue, &stop, command.clone());
                thread::sleep(interval);
            }
        });
    }
}

#[cfg(feature = "serial")]
fn enqueue_command(queue: &CommandQueue, stop: &AtomicBool, command: String) -> Result<(), String> {
    if stop.load(Ordering::Relaxed) {
        return Err("serial writer is closed".to_string());
    }
    let (lock, condvar) = &**queue;
    lock.lock()
        .expect("serial queue mutex poisoned")
        .push_back(command);
    condvar.notify_one();
    Ok(())
}

#[cfg(feature = "serial")]
fn command_bytes(command: &str) -> Vec<u8> {
    command
        .chars()
        .flat_map(|ch| {
            let code = ch as u32;
            if code <= u8::MAX as u32 {
                vec![code as u8]
            } else {
                ch.to_string().into_bytes()
            }
        })
        .collect()
}

#[cfg(feature = "serial")]
fn is_realtime_command(command: &str) -> bool {
    command.chars().count() == 1 && !command.contains('\n')
}

#[cfg(feature = "serial")]
impl Drop for SerialLink {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Relaxed);
        let (_, condvar) = &*self.queue;
        condvar.notify_all();
    }
}

#[cfg(all(test, feature = "serial"))]
mod tests {
    use super::*;

    #[test]
    fn clear_pending_discards_queued_commands_before_reset() {
        let link = SerialLink {
            queue: Arc::new((Mutex::new(VecDeque::new()), Condvar::new())),
            stop: Arc::new(AtomicBool::new(false)),
        };

        link.send_all(vec![
            "G0 X1\n".to_string(),
            "G0 X2\n".to_string(),
            "G0 X3\n".to_string(),
        ])
        .expect("queue commands");
        assert_eq!(pending_count(&link), 3);

        link.clear_pending();
        assert_eq!(pending_count(&link), 0);

        link.send("\u{18}".to_string()).expect("queue reset");
        assert_eq!(pending_count(&link), 1);
    }

    #[test]
    fn realtime_commands_are_written_as_single_bytes() {
        assert_eq!(command_bytes("\u{91}"), vec![0x91]);
        assert_eq!(command_bytes("G0 X1\n"), b"G0 X1\n");
    }

    #[test]
    fn realtime_commands_jump_ahead_of_buffered_gcode() {
        let link = SerialLink {
            queue: Arc::new((Mutex::new(VecDeque::new()), Condvar::new())),
            stop: Arc::new(AtomicBool::new(false)),
        };

        link.send_all(vec![
            "G0 X1\n".to_string(),
            "G0 X2\n".to_string(),
        ])
        .expect("queue gcode");
        link.send_all(vec!["!".to_string(), "~".to_string(), "G0 X3\n".to_string()])
            .expect("queue realtime");

        assert_eq!(
            queued_commands(&link),
            vec![
                "!".to_string(),
                "~".to_string(),
                "G0 X1\n".to_string(),
                "G0 X2\n".to_string(),
                "G0 X3\n".to_string()
            ]
        );
    }

    #[test]
    fn realtime_commands_preserve_order_with_existing_realtime_controls() {
        let link = SerialLink {
            queue: Arc::new((Mutex::new(VecDeque::new()), Condvar::new())),
            stop: Arc::new(AtomicBool::new(false)),
        };

        link.send_all(vec!["~".to_string(), "G0 X1\n".to_string()])
            .expect("queue run");
        link.send_all(vec!["!".to_string()]).expect("queue hold");

        assert_eq!(
            queued_commands(&link),
            vec!["~".to_string(), "!".to_string(), "G0 X1\n".to_string()]
        );
    }

    fn pending_count(link: &SerialLink) -> usize {
        let (lock, _) = &*link.queue;
        lock.lock().expect("serial queue mutex poisoned").len()
    }

    fn queued_commands(link: &SerialLink) -> Vec<String> {
        let (lock, _) = &*link.queue;
        lock.lock()
            .expect("serial queue mutex poisoned")
            .iter()
            .cloned()
            .collect()
    }
}
