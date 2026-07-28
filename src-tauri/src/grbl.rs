use crate::machine::{MachineMode, Vec4};

#[derive(Clone, Debug, PartialEq)]
pub struct ParsedStatus {
    pub mode: MachineMode,
    pub wpos: Option<Vec4>,
    pub mpos: Option<Vec4>,
    pub wco: Option<Vec4>,
    pub feed_rate: Option<f64>,
    pub spindle_speed: Option<f64>,
    pub feed_override: Option<f64>,
    pub rapid_override: Option<f64>,
    pub spindle_override: Option<f64>,
    pub planner_free: Option<usize>,
    pub serial_free: Option<usize>,
    pub pins: Option<String>,
    pub axis_count: Option<u8>,
}

#[derive(Clone, Debug, PartialEq)]
pub enum GrblLine {
    Status(Box<ParsedStatus>),
    ActiveCodes(String),
    Message(String),
    Alarm(String),
    Error(String),
    Ok,
    Config { key: u32, value: f64 },
    Info { key: String, value: String },
    Other(String),
}

pub fn parse_line(line: &str) -> GrblLine {
    let line = line.trim();
    if line == "ok" {
        return GrblLine::Ok;
    }
    if line.starts_with("error:") {
        return GrblLine::Error(line.to_string());
    }
    if line.starts_with("ALARM:") {
        return GrblLine::Alarm(line.to_string());
    }
    if line.starts_with("[MSG:") && line.ends_with(']') {
        return GrblLine::Message(line[5..line.len() - 1].to_string());
    }
    if line.starts_with("[GC:") && line.ends_with(']') {
        return GrblLine::ActiveCodes(line[4..line.len() - 1].to_string());
    }
    if line.starts_with('[') && line.ends_with(']') {
        if let Some((key, value)) = line[1..line.len() - 1].split_once(':') {
            return GrblLine::Info {
                key: key.to_ascii_uppercase(),
                value: value.to_string(),
            };
        }
    }
    if let Some((key, value)) = parse_config(line) {
        return GrblLine::Config { key, value };
    }
    if line.starts_with('<') && line.ends_with('>') {
        return parse_status(&line[1..line.len() - 1])
            .map(Box::new)
            .map(GrblLine::Status)
            .unwrap_or_else(|| GrblLine::Other(line.to_string()));
    }
    GrblLine::Other(line.to_string())
}

fn parse_config(line: &str) -> Option<(u32, f64)> {
    let (key, value) = line.strip_prefix('$')?.split_once('=')?;
    Some((key.parse().ok()?, value.parse().ok()?))
}

pub fn parse_status(body: &str) -> Option<ParsedStatus> {
    let mut parts = body.split('|');
    let mode = parse_mode(parts.next()?)?;
    let mut status = ParsedStatus {
        mode,
        wpos: None,
        mpos: None,
        wco: None,
        feed_rate: None,
        spindle_speed: None,
        feed_override: None,
        rapid_override: None,
        spindle_override: None,
        planner_free: None,
        serial_free: None,
        pins: None,
        axis_count: None,
    };

    for part in parts {
        let Some((key, value)) = part.split_once(':') else {
            continue;
        };
        match key.to_ascii_lowercase().as_str() {
            "wpos" => {
                let (pos, axes) = parse_vec(value)?;
                status.wpos = Some(pos);
                status.axis_count = Some(axes);
            }
            "mpos" => {
                let (pos, axes) = parse_vec(value)?;
                status.mpos = Some(pos);
                status.axis_count = Some(axes);
            }
            "wco" => {
                let (pos, axes) = parse_vec(value)?;
                status.wco = Some(pos);
                status.axis_count = Some(axes);
            }
            "fs" => {
                let vals = parse_numbers(value);
                status.feed_rate = vals.first().copied();
                status.spindle_speed = vals.get(1).copied();
            }
            "f" => {
                status.feed_rate = parse_numbers(value).first().copied();
            }
            "ov" => {
                let vals = parse_numbers(value);
                status.feed_override = vals.first().copied();
                status.rapid_override = vals.get(1).copied();
                status.spindle_override = vals.get(2).copied();
            }
            "bf" => {
                let vals = parse_numbers(value);
                status.planner_free = vals.first().map(|v| *v as usize);
                status.serial_free = vals.get(1).map(|v| *v as usize);
            }
            "pn" => status.pins = Some(value.to_string()),
            _ => {}
        }
    }

    Some(status)
}

fn parse_mode(mode: &str) -> Option<MachineMode> {
    let base = mode.split(':').next().unwrap_or(mode);
    match base {
        "Idle" => Some(MachineMode::Idle),
        "Run" => Some(MachineMode::Running),
        "Jog" => Some(MachineMode::Jogging),
        "Hold" => Some(MachineMode::Hold),
        "Alarm" => Some(MachineMode::Alarm),
        "Door" | "Check" | "Home" | "Sleep" => Some(MachineMode::Hold),
        _ => None,
    }
}

pub fn parse_vec(value: &str) -> Option<(Vec4, u8)> {
    let vals = parse_numbers(value);
    if vals.len() < 3 {
        return None;
    }
    Some((
        Vec4 {
            x: vals[0],
            y: vals[1],
            z: vals[2],
            a: vals.get(3).copied().unwrap_or_default(),
        },
        vals.len().min(u8::MAX as usize) as u8,
    ))
}

fn parse_numbers(value: &str) -> Vec<f64> {
    value
        .split(',')
        .filter_map(|part| part.parse::<f64>().ok())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_grbl_status() {
        let line = "<Idle|MPos:1.000,2.000,3.000|WCO:0.100,0.200,0.300|FS:400,12000|Bf:15,127|Ov:90,50,110|Pn:P>";
        let GrblLine::Status(status) = parse_line(line) else {
            panic!("expected status");
        };
        assert_eq!(status.mode, MachineMode::Idle);
        assert_eq!(status.mpos.unwrap().x, 1.0);
        assert_eq!(status.wco.unwrap().z, 0.3);
        assert_eq!(status.feed_rate, Some(400.0));
        assert_eq!(status.spindle_speed, Some(12000.0));
        assert_eq!(status.planner_free, Some(15));
        assert_eq!(status.serial_free, Some(127));
        assert_eq!(status.pins, Some("P".to_string()));
    }

    #[test]
    fn parses_four_axis_status() {
        let GrblLine::Status(status) =
            parse_line("<Idle|WPos:1.000,2.000,3.000,4.000|WCO:0.000,0.000,0.000,0.000>")
        else {
            panic!("expected status");
        };
        assert_eq!(status.axis_count, Some(4));
        assert_eq!(status.wpos.unwrap().a, 4.0);
    }

    #[test]
    fn parses_messages_and_config() {
        assert_eq!(parse_line("ok"), GrblLine::Ok);
        assert_eq!(
            parse_line("[GC:G0 G54]"),
            GrblLine::ActiveCodes("G0 G54".into())
        );
        assert_eq!(
            parse_line("[MSG:Reset to continue]"),
            GrblLine::Message("Reset to continue".into())
        );
        assert_eq!(
            parse_line("$110=5000.000"),
            GrblLine::Config {
                key: 110,
                value: 5000.0
            }
        );
    }

    #[test]
    fn parses_controller_info_records() {
        assert_eq!(
            parse_line("[FIRMWARE:grblHAL]"),
            GrblLine::Info {
                key: "FIRMWARE".to_string(),
                value: "grblHAL".to_string()
            }
        );
        assert_eq!(
            parse_line("[NEWOPT:ENUMS,RT+,HOME,NOPROBE,ES,SED,RTC,TMC=7]"),
            GrblLine::Info {
                key: "NEWOPT".to_string(),
                value: "ENUMS,RT+,HOME,NOPROBE,ES,SED,RTC,TMC=7".to_string()
            }
        );
    }
}
