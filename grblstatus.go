package main

import "time"

type GrblStatus struct {
	PortName         string
	Ready            bool
	Closed           bool
	Status           string
	Wco              V4d
	Mpos             V4d
	Wpos             V4d
	Dtg              V4d // TODO: how can we calculate this?
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
