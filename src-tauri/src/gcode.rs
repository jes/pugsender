use crate::machine::Vec4;
use serde::{Deserialize, Serialize};
use std::f64::consts::TAU;

#[derive(Clone, Debug, Default, Deserialize, Serialize, PartialEq)]
pub struct Toolpath {
    pub points: Vec<Vec4>,
    pub point_source_lines: Vec<Option<usize>>,
    pub min: Vec4,
    pub max: Vec4,
    pub estimated_seconds: f64,
    pub line_estimated_seconds: Vec<f64>,
    pub remaining_seconds_by_line: Vec<f64>,
}

#[derive(Clone, Copy, Debug, PartialEq)]
enum DistanceMode {
    Absolute,
    Relative,
}

#[derive(Clone, Copy, Debug, PartialEq)]
enum UnitMode {
    Mm,
    Inches,
}

impl UnitMode {
    fn scale(self) -> f64 {
        match self {
            UnitMode::Mm => 1.0,
            UnitMode::Inches => 25.4,
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq)]
enum Plane {
    XY,
    XZ,
    YZ,
}

#[derive(Clone, Copy, Debug, PartialEq)]
enum RetractMode {
    Initial,
    RPlane,
}

pub fn build_toolpath(lines: &[String]) -> Toolpath {
    let mut state = PathState {
        pos: Vec4::default(),
        g92_offset: Vec4::default(),
        motion: None,
        distance_mode: DistanceMode::Absolute,
        unit_mode: UnitMode::Mm,
        plane: Plane::XY,
        retract_mode: RetractMode::Initial,
        feed_rate: 300.0,
        rapid_rate: 3000.0,
        estimated_seconds: 0.0,
        line_estimated_seconds: vec![0.0; lines.len()],
        points: vec![Vec4::default()],
        point_source_lines: vec![None],
    };

    for (source_line, line) in lines.iter().enumerate() {
        if state.apply_line(line, source_line) {
            break;
        }
    }

    let mut path = Toolpath {
        points: state.points,
        point_source_lines: state.point_source_lines,
        min: Vec4::default(),
        max: Vec4::default(),
        estimated_seconds: state.estimated_seconds,
        line_estimated_seconds: state.line_estimated_seconds,
        remaining_seconds_by_line: Vec::new(),
    };
    path.remaining_seconds_by_line = remaining_seconds_by_line(&path.line_estimated_seconds);
    if let Some(first) = path.points.first().copied() {
        path.min = first;
        path.max = first;
        for point in &path.points {
            path.min.x = path.min.x.min(point.x);
            path.min.y = path.min.y.min(point.y);
            path.min.z = path.min.z.min(point.z);
            path.min.a = path.min.a.min(point.a);
            path.max.x = path.max.x.max(point.x);
            path.max.y = path.max.y.max(point.y);
            path.max.z = path.max.z.max(point.z);
            path.max.a = path.max.a.max(point.a);
        }
    }
    path
}

struct PathState {
    pos: Vec4,
    g92_offset: Vec4,
    motion: Option<i32>,
    distance_mode: DistanceMode,
    unit_mode: UnitMode,
    plane: Plane,
    retract_mode: RetractMode,
    feed_rate: f64,
    rapid_rate: f64,
    estimated_seconds: f64,
    line_estimated_seconds: Vec<f64>,
    points: Vec<Vec4>,
    point_source_lines: Vec<Option<usize>>,
}

impl PathState {
    fn apply_line(&mut self, line: &str, source_line: usize) -> bool {
        let words = parse_words(line);
        if words.is_empty() {
            return false;
        }

        let program_end = is_program_end(&words);
        let mut has_motion_code = false;
        for (letter, value) in &words {
            if *letter == 'G' {
                match *value as i32 {
                    0..=3 => {
                        self.motion = Some(*value as i32);
                        has_motion_code = true;
                    }
                    4 => {
                        self.add_dwell(&words, source_line);
                        return program_end;
                    }
                    17 => self.plane = Plane::XY,
                    18 => self.plane = Plane::XZ,
                    19 => self.plane = Plane::YZ,
                    20 => self.unit_mode = UnitMode::Inches,
                    21 => self.unit_mode = UnitMode::Mm,
                    90 => self.distance_mode = DistanceMode::Absolute,
                    91 => self.distance_mode = DistanceMode::Relative,
                    80 => self.motion = None,
                    73 | 81..=89 => {
                        self.motion = Some(*value as i32);
                        has_motion_code = true;
                    }
                    92 => {
                        if g92_is_clear(*value) {
                            self.g92_offset = Vec4::default();
                        } else {
                            self.set_g92_offset(&words);
                        }
                        return program_end;
                    }
                    98 => self.retract_mode = RetractMode::Initial,
                    99 => self.retract_mode = RetractMode::RPlane,
                    _ => {}
                }
            } else if *letter == 'F' && *value > 0.0 {
                self.feed_rate = *value * self.unit_mode.scale();
            }
        }

        let Some(motion) = self.motion else {
            return program_end;
        };

        if !has_motion_code && !has_axis_words(&words) {
            return program_end;
        }
        let target = self.target_from_words(&words);
        if target == self.pos {
            return program_end;
        }

        if is_canned_cycle(motion) {
            self.add_drill_cycle(target, &words, motion, source_line);
        } else if motion == 2 || motion == 3 {
            self.add_arc(target, &words, motion == 3, source_line);
        } else {
            let rate = if motion == 0 {
                self.rapid_rate
            } else {
                self.feed_rate
            };
            self.push_point(target, rate, Some(source_line));
        }
        self.pos = target;
        program_end
    }

    fn target_from_words(&self, words: &[(char, f64)]) -> Vec4 {
        let mut target = self.pos;
        for (letter, value) in words {
            match *letter {
                'X' => set_axis(
                    &mut target.x,
                    self.pos.x,
                    *value * self.unit_mode.scale() + self.g92_offset.x,
                    self.distance_mode,
                ),
                'Y' => set_axis(
                    &mut target.y,
                    self.pos.y,
                    *value * self.unit_mode.scale() + self.g92_offset.y,
                    self.distance_mode,
                ),
                'Z' => set_axis(
                    &mut target.z,
                    self.pos.z,
                    *value * self.unit_mode.scale() + self.g92_offset.z,
                    self.distance_mode,
                ),
                'A' => set_axis(
                    &mut target.a,
                    self.pos.a,
                    *value + self.g92_offset.a,
                    self.distance_mode,
                ),
                _ => {}
            }
        }
        target
    }

    fn set_g92_offset(&mut self, words: &[(char, f64)]) {
        for (letter, value) in words {
            match *letter {
                'X' => self.g92_offset.x = self.pos.x - *value * self.unit_mode.scale(),
                'Y' => self.g92_offset.y = self.pos.y - *value * self.unit_mode.scale(),
                'Z' => self.g92_offset.z = self.pos.z - *value * self.unit_mode.scale(),
                'A' => self.g92_offset.a = self.pos.a - *value,
                _ => {}
            }
        }
    }

    fn add_dwell(&mut self, words: &[(char, f64)], source_line: usize) {
        if let Some(seconds) = word_value(words, 'P').or_else(|| word_value(words, 'X')) {
            self.add_estimated_seconds(seconds.max(0.0), Some(source_line));
        }
    }

    fn add_drill_cycle(
        &mut self,
        target: Vec4,
        words: &[(char, f64)],
        motion: i32,
        source_line: usize,
    ) {
        let initial_z = self.pos.z;
        let retract_z = word_value(words, 'R')
            .map(|r| axis_value(self.pos.z, r * self.unit_mode.scale(), self.distance_mode))
            .unwrap_or(self.pos.z);
        let approach = Vec4 {
            x: target.x,
            y: target.y,
            z: retract_z,
            a: target.a,
        };
        let bottom = Vec4 {
            x: target.x,
            y: target.y,
            z: target.z,
            a: target.a,
        };
        if approach != self.pos {
            self.push_point(approach, self.rapid_rate, Some(source_line));
        }
        if motion == 73 || motion == 83 {
            self.add_peck_moves(approach, bottom, words, source_line);
        } else if bottom != approach {
            self.push_point(bottom, self.feed_rate, Some(source_line));
        }
        if matches!(motion, 82 | 86 | 88 | 89) {
            self.add_dwell(words, source_line);
        }

        let final_retract_z = match self.retract_mode {
            RetractMode::Initial => initial_z.max(retract_z),
            RetractMode::RPlane => retract_z,
        };
        let retract = Vec4 {
            z: final_retract_z,
            ..approach
        };
        if self.points.last().copied() != Some(retract) {
            self.push_point(retract, self.rapid_rate, Some(source_line));
        }
    }

    fn add_peck_moves(
        &mut self,
        approach: Vec4,
        bottom: Vec4,
        words: &[(char, f64)],
        source_line: usize,
    ) {
        let peck = word_value(words, 'Q')
            .map(|q| (q * self.unit_mode.scale()).abs())
            .filter(|q| *q > f64::EPSILON)
            .unwrap_or_else(|| (approach.z - bottom.z).abs());
        let direction = if bottom.z <= approach.z { -1.0 } else { 1.0 };
        let mut current_z = approach.z;

        while (bottom.z - current_z).abs() > f64::EPSILON {
            let next_z = if direction < 0.0 {
                (current_z - peck).max(bottom.z)
            } else {
                (current_z + peck).min(bottom.z)
            };
            let peck_bottom = Vec4 {
                z: next_z,
                ..approach
            };
            self.push_point(peck_bottom, self.feed_rate, Some(source_line));
            current_z = next_z;
            if (bottom.z - current_z).abs() > f64::EPSILON {
                self.push_point(approach, self.rapid_rate, Some(source_line));
            }
        }
    }

    fn add_arc(&mut self, target: Vec4, words: &[(char, f64)], ccw: bool, source_line: usize) {
        let Some((start_u, start_v, end_u, end_v, centre_u, centre_v, radius)) =
            self.arc_geometry(target, words, ccw)
        else {
            self.push_point(target, self.feed_rate, Some(source_line));
            return;
        };
        if radius <= f64::EPSILON {
            self.push_point(target, self.feed_rate, Some(source_line));
            return;
        }

        let start_angle = (start_v - centre_v).atan2(start_u - centre_u);
        let end_angle = (end_v - centre_v).atan2(end_u - centre_u);
        let mut sweep = end_angle - start_angle;
        if ccw && sweep <= 0.0 {
            sweep += TAU;
        } else if !ccw && sweep >= 0.0 {
            sweep -= TAU;
        }
        let steps = ((sweep.abs() * radius) / 0.5).ceil().clamp(8.0, 240.0) as usize;
        for step in 1..=steps {
            let t = step as f64 / steps as f64;
            let angle = start_angle + sweep * t;
            let u = centre_u + radius * angle.cos();
            let v = centre_v + radius * angle.sin();
            self.push_point(
                self.point_from_plane(target, u, v, t),
                self.feed_rate,
                Some(source_line),
            );
        }
    }

    fn push_point(&mut self, point: Vec4, feed_rate: f64, source_line: Option<usize>) {
        if let Some(previous) = self.points.last().copied() {
            let distance = ((point.x - previous.x).powi(2)
                + (point.y - previous.y).powi(2)
                + (point.z - previous.z).powi(2))
            .sqrt();
            if feed_rate > 0.0 {
                self.add_estimated_seconds(distance / feed_rate * 60.0, source_line);
            }
        }
        self.points.push(point);
        self.point_source_lines.push(source_line);
    }

    fn add_estimated_seconds(&mut self, seconds: f64, source_line: Option<usize>) {
        self.estimated_seconds += seconds;
        if let Some(source_line) = source_line {
            if let Some(line_seconds) = self.line_estimated_seconds.get_mut(source_line) {
                *line_seconds += seconds;
            }
        }
    }

    fn arc_geometry(
        &self,
        target: Vec4,
        words: &[(char, f64)],
        ccw: bool,
    ) -> Option<(f64, f64, f64, f64, f64, f64, f64)> {
        let (start_u, start_v) = self.plane_coords(self.pos);
        let (end_u, end_v) = self.plane_coords(target);

        if let Some(r) = word_value(words, 'R') {
            return arc_from_radius(
                start_u,
                start_v,
                end_u,
                end_v,
                r * self.unit_mode.scale(),
                ccw,
            );
        }

        let (offset_u, offset_v) = match self.plane {
            Plane::XY => (word_value(words, 'I'), word_value(words, 'J')),
            Plane::XZ => (word_value(words, 'I'), word_value(words, 'K')),
            Plane::YZ => (word_value(words, 'J'), word_value(words, 'K')),
        };
        let offset_u = offset_u.unwrap_or(0.0) * self.unit_mode.scale();
        let offset_v = offset_v.unwrap_or(0.0) * self.unit_mode.scale();
        let centre_u = start_u + offset_u;
        let centre_v = start_v + offset_v;
        let radius = (offset_u * offset_u + offset_v * offset_v).sqrt();
        Some((start_u, start_v, end_u, end_v, centre_u, centre_v, radius))
    }

    fn plane_coords(&self, point: Vec4) -> (f64, f64) {
        match self.plane {
            Plane::XY => (point.x, point.y),
            Plane::XZ => (point.x, point.z),
            Plane::YZ => (point.y, point.z),
        }
    }

    fn point_from_plane(&self, target: Vec4, u: f64, v: f64, t: f64) -> Vec4 {
        match self.plane {
            Plane::XY => Vec4 {
                x: u,
                y: v,
                z: self.pos.z + (target.z - self.pos.z) * t,
                a: self.pos.a + (target.a - self.pos.a) * t,
            },
            Plane::XZ => Vec4 {
                x: u,
                y: self.pos.y + (target.y - self.pos.y) * t,
                z: v,
                a: self.pos.a + (target.a - self.pos.a) * t,
            },
            Plane::YZ => Vec4 {
                x: self.pos.x + (target.x - self.pos.x) * t,
                y: u,
                z: v,
                a: self.pos.a + (target.a - self.pos.a) * t,
            },
        }
    }
}

fn set_axis(axis: &mut f64, current: f64, value: f64, mode: DistanceMode) {
    *axis = axis_value(current, value, mode);
}

fn axis_value(current: f64, value: f64, mode: DistanceMode) -> f64 {
    match mode {
        DistanceMode::Absolute => value,
        DistanceMode::Relative => current + value,
    }
}

fn arc_from_radius(
    start_u: f64,
    start_v: f64,
    end_u: f64,
    end_v: f64,
    radius: f64,
    ccw: bool,
) -> Option<(f64, f64, f64, f64, f64, f64, f64)> {
    let chord_u = end_u - start_u;
    let chord_v = end_v - start_v;
    let chord_len = (chord_u * chord_u + chord_v * chord_v).sqrt();
    let abs_radius = radius.abs();
    if chord_len <= f64::EPSILON || abs_radius < chord_len / 2.0 {
        return None;
    }
    let midpoint_u = (start_u + end_u) / 2.0;
    let midpoint_v = (start_v + end_v) / 2.0;
    let height = (abs_radius * abs_radius - (chord_len / 2.0).powi(2)).sqrt();
    let normal_u = -chord_v / chord_len;
    let normal_v = chord_u / chord_len;

    let candidates = [
        (
            midpoint_u + normal_u * height,
            midpoint_v + normal_v * height,
        ),
        (
            midpoint_u - normal_u * height,
            midpoint_v - normal_v * height,
        ),
    ];
    let (centre_u, centre_v) = candidates.into_iter().find(|(u, v)| {
        let start_angle = (start_v - v).atan2(start_u - u);
        let end_angle = (end_v - v).atan2(end_u - u);
        let mut sweep = end_angle - start_angle;
        if ccw && sweep <= 0.0 {
            sweep += TAU;
        } else if !ccw && sweep >= 0.0 {
            sweep -= TAU;
        }
        (radius >= 0.0 && sweep.abs() <= std::f64::consts::PI)
            || (radius < 0.0 && sweep.abs() > std::f64::consts::PI)
    })?;
    Some((
        start_u, start_v, end_u, end_v, centre_u, centre_v, abs_radius,
    ))
}

fn word_value(words: &[(char, f64)], letter: char) -> Option<f64> {
    words
        .iter()
        .find_map(|(word_letter, value)| (*word_letter == letter).then_some(*value))
}

fn is_canned_cycle(motion: i32) -> bool {
    motion == 73 || (81..=89).contains(&motion)
}

fn is_program_end(words: &[(char, f64)]) -> bool {
    words
        .iter()
        .any(|(letter, value)| *letter == 'M' && (approx(*value, 2.0) || approx(*value, 30.0)))
}

fn has_axis_words(words: &[(char, f64)]) -> bool {
    words
        .iter()
        .any(|(letter, _)| matches!(*letter, 'X' | 'Y' | 'Z' | 'A'))
}

fn g92_is_clear(value: f64) -> bool {
    (value - 92.1).abs() < 0.0001 || (value - 92.2).abs() < 0.0001
}

fn approx(a: f64, b: f64) -> bool {
    (a - b).abs() < 0.0001
}

fn parse_words(line: &str) -> Vec<(char, f64)> {
    let line = strip_comments(line).to_ascii_uppercase();
    let bytes = line.as_bytes();
    let mut words = Vec::new();
    let mut i = 0;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if !ch.is_ascii_alphabetic() {
            i += 1;
            continue;
        }
        i += 1;
        let start = i;
        while i < bytes.len() {
            let next = bytes[i] as char;
            if next.is_ascii_digit() || matches!(next, '.' | '-' | '+') {
                i += 1;
            } else {
                break;
            }
        }
        if let Ok(value) = line[start..i].parse::<f64>() {
            words.push((ch, value));
        }
    }
    words
}

fn strip_comments(line: &str) -> String {
    let mut out = String::new();
    let mut paren_depth = 0;
    for ch in line.chars() {
        if ch == ';' && paren_depth == 0 {
            break;
        }
        if ch == '(' {
            paren_depth += 1;
            continue;
        }
        if ch == ')' && paren_depth > 0 {
            paren_depth -= 1;
            continue;
        }
        if paren_depth == 0 {
            out.push(ch);
        }
    }
    out
}

fn remaining_seconds_by_line(line_estimated_seconds: &[f64]) -> Vec<f64> {
    let mut remaining = vec![0.0; line_estimated_seconds.len() + 1];
    let mut total = 0.0;
    for index in (0..line_estimated_seconds.len()).rev() {
        total += line_estimated_seconds[index];
        remaining[index] = total;
    }
    remaining
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_lowercase_without_spaces() {
        let path = build_toolpath(&["g0x1y2".to_string(), "g1x3".to_string()]);
        assert_eq!(path.points.last().unwrap().x, 3.0);
        assert_eq!(path.points.last().unwrap().y, 2.0);
    }

    #[test]
    fn supports_relative_moves() {
        let path = build_toolpath(&["G90 G0 X1 Y1".to_string(), "G91 G1 X2 Y-1".to_string()]);
        assert_eq!(path.points.last().unwrap().x, 3.0);
        assert_eq!(path.points.last().unwrap().y, 0.0);
    }

    #[test]
    fn stops_preview_at_program_end() {
        let path = build_toolpath(&["G0 X1".to_string(), "M30".to_string(), "G0 X99".to_string()]);
        assert_eq!(path.points.last().unwrap().x, 1.0);
        assert_eq!(path.point_source_lines.last().copied(), Some(Some(0)));
    }

    #[test]
    fn supports_g92_temporary_coordinate_offsets() {
        let path = build_toolpath(&[
            "G0 X10 Y10".to_string(),
            "G92 X0 Y0".to_string(),
            "G1 X5 Y0".to_string(),
            "G92.1".to_string(),
            "G1 X0 Y0".to_string(),
        ]);
        assert_eq!(path.points.len(), 4);
        assert_eq!(path.points[1].x, 10.0);
        assert_eq!(path.points[1].y, 10.0);
        assert_eq!(path.points[2].x, 15.0);
        assert_eq!(path.points[2].y, 10.0);
        assert_eq!(path.points[3].x, 0.0);
        assert_eq!(path.points[3].y, 0.0);
    }

    #[test]
    fn adds_dwell_to_cycle_time_without_motion() {
        let path = build_toolpath(&["G0 X10".to_string(), "G4 P2.5".to_string()]);
        assert_eq!(path.points.len(), 2);
        assert!((path.estimated_seconds - 2.7).abs() < 0.01);
        assert!((path.line_estimated_seconds[0] - 0.2).abs() < 0.01);
        assert!((path.line_estimated_seconds[1] - 2.5).abs() < 0.01);
    }

    #[test]
    fn records_source_line_for_each_generated_point() {
        let path = build_toolpath(&[
            "(setup)".to_string(),
            "G0 X1".to_string(),
            "G3 X0 Y1 I-1 J0".to_string(),
        ]);
        assert_eq!(path.points.len(), path.point_source_lines.len());
        assert_eq!(path.point_source_lines.first().copied(), Some(None));
        assert_eq!(path.point_source_lines.get(1).copied(), Some(Some(1)));
        assert_eq!(path.point_source_lines.last().copied(), Some(Some(2)));
    }

    #[test]
    fn computes_remaining_estimates_by_source_line() {
        let path = build_toolpath(&[
            "G1 F60 X1".to_string(),
            "G1 X3".to_string(),
            "G4 P5".to_string(),
        ]);
        assert!((path.line_estimated_seconds[0] - 1.0).abs() < 0.01);
        assert!((path.line_estimated_seconds[1] - 2.0).abs() < 0.01);
        assert!((path.line_estimated_seconds[2] - 5.0).abs() < 0.01);
        assert!((path.remaining_seconds_by_line[0] - 8.0).abs() < 0.01);
        assert!((path.remaining_seconds_by_line[1] - 7.0).abs() < 0.01);
        assert!((path.remaining_seconds_by_line[2] - 5.0).abs() < 0.01);
        assert_eq!(path.remaining_seconds_by_line[3], 0.0);
    }

    #[test]
    fn approximates_arcs() {
        let path = build_toolpath(&["G0 X1 Y0".to_string(), "G3 X0 Y1 I-1 J0".to_string()]);
        assert!(path.points.len() > 3);
        assert!((path.points.last().unwrap().x - 0.0).abs() < 0.001);
        assert!((path.points.last().unwrap().y - 1.0).abs() < 0.001);
    }

    #[test]
    fn scales_inches_to_mm() {
        let path = build_toolpath(&["G20 G0 X1 Y0.5".to_string()]);
        assert!((path.points.last().unwrap().x - 25.4).abs() < 0.001);
        assert!((path.points.last().unwrap().y - 12.7).abs() < 0.001);
    }

    #[test]
    fn supports_xz_plane_arcs() {
        let path = build_toolpath(&["G18 G0 X1 Z0".to_string(), "G3 X0 Z1 I-1 K0".to_string()]);
        assert!(path.points.len() > 3);
        assert!((path.points.last().unwrap().x - 0.0).abs() < 0.001);
        assert!((path.points.last().unwrap().z - 1.0).abs() < 0.001);
    }

    #[test]
    fn supports_radius_arcs() {
        let path = build_toolpath(&["G0 X1 Y0".to_string(), "G3 X0 Y1 R1".to_string()]);
        assert!(path.points.len() > 3);
        assert!((path.points.last().unwrap().x - 0.0).abs() < 0.001);
        assert!((path.points.last().unwrap().y - 1.0).abs() < 0.001);
    }

    #[test]
    fn previews_g81_drilling_cycles() {
        let path = build_toolpath(&[
            "G0 Z5".to_string(),
            "G81 X10 Y20 Z-2 R1".to_string(),
            "G80".to_string(),
            "X30 Y40".to_string(),
        ]);
        assert!(path
            .points
            .iter()
            .any(|point| point.x == 10.0 && point.y == 20.0 && point.z == -2.0));
        assert!(path
            .points
            .iter()
            .any(|point| point.x == 10.0 && point.y == 20.0 && point.z == 1.0));
        assert_eq!(path.points.last().unwrap().x, 10.0);
        assert_eq!(path.points.last().unwrap().y, 20.0);
    }

    #[test]
    fn previews_g82_dwell_drilling_cycles() {
        let path = build_toolpath(&[
            "G0 Z5".to_string(),
            "G99 G82 X10 Y20 Z-2 R1 P0.5".to_string(),
        ]);
        assert!(path
            .points
            .iter()
            .any(|point| point.x == 10.0 && point.y == 20.0 && point.z == -2.0));
        assert_eq!(path.points.last().unwrap().z, 1.0);
    }

    #[test]
    fn previews_g83_peck_drilling_cycles() {
        let path = build_toolpath(&["G99 G83 X0 Y0 Z-3 R0 Q1".to_string()]);
        let z_values: Vec<f64> = path.points.iter().map(|point| point.z).collect();
        assert!(z_values.contains(&-1.0));
        assert!(z_values.contains(&-2.0));
        assert!(z_values.contains(&-3.0));
        assert_eq!(path.points.last().unwrap().z, 0.0);
    }

    #[test]
    fn previews_g73_chip_break_cycles() {
        let path = build_toolpath(&["G99 G73 X0 Y0 Z-3 R0 Q1".to_string()]);
        let z_values: Vec<f64> = path.points.iter().map(|point| point.z).collect();
        assert!(z_values.contains(&-1.0));
        assert!(z_values.contains(&-2.0));
        assert!(z_values.contains(&-3.0));
        assert_eq!(path.points.last().unwrap().z, 0.0);
    }

    #[test]
    fn previews_boring_cycles_and_dwell_time() {
        let path = build_toolpath(&["G99 G89 X5 Y6 Z-2 R1 P1.5 F60".to_string()]);
        assert!(path
            .points
            .iter()
            .any(|point| point.x == 5.0 && point.y == 6.0 && point.z == -2.0));
        assert_eq!(path.points.last().unwrap().z, 1.0);
        assert!(path.estimated_seconds > 4.0);

        let g85 = build_toolpath(&["G85 X1 Y2 Z-1 R0".to_string()]);
        assert_eq!(g85.points.last().unwrap().z, 0.0);
    }

    #[test]
    fn supports_initial_plane_retract_mode() {
        let path = build_toolpath(&["G0 Z5".to_string(), "G98 G81 X1 Z-1 R1".to_string()]);
        assert_eq!(path.points.last().unwrap().z, 5.0);
    }

    #[test]
    fn estimates_cycle_time_from_motion_and_feed() {
        let path = build_toolpath(&[
            "G0 X10".to_string(),
            "G1 F60 X20".to_string(),
            "F120 X40".to_string(),
        ]);
        assert!((path.estimated_seconds - 20.2).abs() < 0.01);
    }

}
