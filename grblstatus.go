package main

import (
	"fmt"
	"time"
)

type GrblStatus struct {
	PortName         string
	Ready            bool
	Closed           bool
	Status           string
	Wco              V4d
	Mpos             V4d
	Wpos             V4d
	Vel              V4d
	PlannerSize      int
	PlannerFree      int
	SerialSize       int
	SerialFree       int
	SpindleCw        bool
	SpindleCcw       bool
	FloodCoolant     bool
	MistCoolant      bool
	FeedOverride     float64
	RapidOverride    float64
	SpindleOverride  float64
	FeedRate         float64
	SpindleSpeed     float64
	Pn               string
	Probe            bool
	UpdateTime       time.Time
	GCodes           string
	GrblConfig       map[int]float64
	WaitingForGCodes bool
	Has4thAxis       bool
}

func DefaultGrblStatus() GrblStatus {
	return GrblStatus{
		PortName:        "/dev/null",
		Closed:          true,
		Status:          "Connecting",
		SerialFree:      128,
		GrblConfig:      make(map[int]float64),
		FeedOverride:    100,
		RapidOverride:   100,
		SpindleOverride: 100,
	}
}

// extrapolated Wpos
func (gs GrblStatus) WposExt() V4d {
	dt := time.Now().Sub(gs.UpdateTime)
	return gs.Wpos.Add(gs.Vel.Mul(dt.Minutes()))
}

// extrapolated Mpos
func (gs GrblStatus) MposExt() V4d {
	dt := time.Now().Sub(gs.UpdateTime)
	return gs.Mpos.Add(gs.Vel.Mul(dt.Minutes()))
}

func (gs GrblStatus) String() string {
	wposStr := fmt.Sprintf("%.3f,%.3f,%.3f", gs.Wpos.X, gs.Wpos.Y, gs.Wpos.Z)
	wcoStr := fmt.Sprintf("%.3f,%.3f,%.3f", gs.Wco.X, gs.Wco.Y, gs.Wco.Z)
	if gs.Has4thAxis {
		wposStr = fmt.Sprintf("%.3f,%.3f,%.3f,%.3f", gs.Wpos.X, gs.Wpos.Y, gs.Wpos.Z, gs.Wpos.A)
		wcoStr = fmt.Sprintf("%.3f,%.3f,%.3f,%.3f", gs.Wco.X, gs.Wco.Y, gs.Wco.Z, gs.Wco.A)
	}
	bfStr := fmt.Sprintf("%d,%d", gs.PlannerFree, gs.SerialFree)
	fsStr := fmt.Sprintf("%d,%d", int(gs.FeedRate), int(gs.SpindleSpeed))
	ovStr := fmt.Sprintf("%d,%d,%d", int(gs.FeedOverride), int(gs.RapidOverride), int(gs.SpindleOverride))
	aStr := ""
	if gs.SpindleCw {
		aStr += "S"
	}
	if gs.SpindleCcw {
		aStr += "C"
	}
	if gs.FloodCoolant {
		aStr += "F"
	}
	if gs.MistCoolant {
		aStr += "M"
	}
	pnStr := ""
	if gs.Probe {
		pnStr = "P"
	}
	return "<" + gs.Status + "|WPos:" + wposStr + "|WCO:" + wcoStr + "|FS:" + fsStr + "|Bf:" + bfStr + "|Ov:" + ovStr + "|A:" + aStr + "|Pn:" + pnStr + ">"
}
