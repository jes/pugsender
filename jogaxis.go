package main

import (
	"fmt"
	"math"
	"time"
)

type JogAxis struct {
	LastCommand          time.Time
	LastMotion           time.Time
	LastPos              float64
	LastVel              float64
	IncrementalTarget    float64
	Target               float64
	MetIncrementalTarget bool
	UpKeyHeld            bool
	DownKeyHeld          bool
	Idle                 bool
	NeedsCommand         bool
}

type JogAxis4d struct {
	X JogAxis
	Y JogAxis
	Z JogAxis
	A JogAxis
}

func (j *JogAxis4d) Update(pos V4d, vel V4d) {
	j.X.Update(pos.X, vel.X)
	j.Y.Update(pos.Y, vel.Y)
	j.Z.Update(pos.Z, vel.Z)
	j.A.Update(pos.A, vel.A)
}

func (j *JogAxis4d) StepContinuous(dist float64) {
	for _, axis := range j.SelectAll() {
		if axis.UpKeyHeld {
			axis.AddContinuous(dist)
		}
		if axis.DownKeyHeld {
			axis.AddContinuous(-dist)
		}
	}
}

func (j *JogAxis4d) SentCommand() {
	for _, axis := range j.SelectAll() {
		axis.NeedsCommand = false
	}
}

func (j *JogAxis4d) WasCancelled() {
	for _, axis := range j.SelectAll() {
		axis.WasCancelled()
	}
}

func (j *JogAxis4d) JogCommand() string {
	return j.X.JogCommand("X") + j.Y.JogCommand("Y") + j.Z.JogCommand("Z") + j.A.JogCommand("A")
}

func (j *JogAxis4d) SelectAll() []*JogAxis {
	return []*JogAxis{&j.X, &j.Y, &j.Z, &j.A}
}

func (j *JogAxis4d) Select(axis string) *JogAxis {
	if axis == "X" {
		return &j.X
	} else if axis == "Y" {
		return &j.Y
	} else if axis == "Z" {
		return &j.Z
	} else if axis == "A" {
		return &j.A
	} else {
		panic(fmt.Sprintf("JogAxis4d.SelectAxis: '%s' is illegal axis", axis))
	}
}

func (j *JogAxis) Update(pos float64, vel float64) {
	eps := 0.0001

	// the incremental target has been met if we moved past it, or we're within eps of it
	if math.Signbit(pos-j.IncrementalTarget) != math.Signbit(j.LastPos-j.IncrementalTarget) || math.Abs(pos-j.IncrementalTarget) < eps {
		j.MetIncrementalTarget = true
	}

	j.LastPos = pos
	j.LastVel = vel

	gracePeriod := 500 * time.Millisecond
	now := time.Now()

	if math.Abs(vel) > eps {
		j.LastMotion = now
	}

	// reset target if the axis is stationary & uncommanded for "gracePeriod"
	if !j.UpKeyHeld && !j.DownKeyHeld && now.Sub(j.LastCommand) > gracePeriod && now.Sub(j.LastMotion) > gracePeriod {
		j.IncrementalTarget = pos
		j.MetIncrementalTarget = true
		j.Target = pos
		j.Idle = true
	} else {
		j.Idle = false
	}
}

func (j *JogAxis) AddIncremental(dist float64) {
	j.IncrementalTarget += dist
	fmt.Printf("inc = %.3f\n", j.IncrementalTarget)
	if j.Idle || !j.MetIncrementalTarget {
		// if the axis is idle, then the true target is the incremental target
		j.SetTarget(j.IncrementalTarget)
		j.MetIncrementalTarget = false
	} else {
		// if the axis is not idle, then update the true target if the new incremental
		// target is further than the actual jog target and in the same direction
		delta := j.Target - j.LastPos
		incDelta := j.IncrementalTarget - j.LastPos
		targetDir := math.Signbit(delta)
		incTargetDir := math.Signbit(incDelta)
		targetDist := math.Abs(delta)
		incTargetDist := math.Abs(incDelta)
		fmt.Println(j.Target, j.IncrementalTarget, j.LastPos, delta, incDelta, targetDir, incTargetDir, targetDist, incTargetDist)
		eps := 0.001
		if targetDist < eps || (incTargetDir == targetDir && incTargetDist > targetDist) {
			j.SetTarget(j.IncrementalTarget)
			j.MetIncrementalTarget = false
		}
	}
}

func (j *JogAxis) AddContinuous(dist float64) {
	j.SetTarget(j.Target + dist)
}

func (j *JogAxis) SetTarget(v float64) {
	fmt.Printf("target = %.3f\n", v)
	j.Target = v
	j.NeedsCommand = true
	j.LastCommand = time.Now()
}

func (j *JogAxis) WasCancelled() {
	if !j.MetIncrementalTarget || j.UpKeyHeld || j.DownKeyHeld {
		j.NeedsCommand = true
		j.LastCommand = time.Now()
	}
}

func (j *JogAxis) JogCommand(prefix string) string {
	if j.NeedsCommand {
		return fmt.Sprintf("%s%.3f", prefix, j.Target)
	} else {
		return ""
	}
}
