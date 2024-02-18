package main

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func (a *App) LayoutStatusBar(gtx C) D {
	// TODO: stop hard-coding colours everywhere, make the app have a theme that is more expansive than material.Theme
	return LayoutColour(gtx, grey(32), func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx C) D {
				return drawImage(gtx, a.img, float64(a.th.TextSize)/16.0)
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.mode.String()).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.gs.PortName).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.LayoutBufferState),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(fmt.Sprintf("Pn:%s", a.gs.Pn)).Layout),
		)
	})
}

func (a *App) LayoutBufferState(gtx C) D {
	th := material.NewTheme()
	th.TextSize = a.th.TextSize
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			// planner buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.gs.PlannerSize), float64(a.gs.PlannerFree)), th, "PLAN")
		}),
		layout.Rigid(func(gtx C) D {
			// serial buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.gs.SerialSize), float64(a.gs.SerialFree)), th, "SER")
		}),
	)
}

// clamp1 limits v to range [0..1].
func clamp1(v float64) float64 {
	if v >= 1 {
		return 1
	} else if v <= 0 {
		return 0
	} else {
		return v
	}
}

func utilisation(size float64, free float64) float64 {
	if size == 0 {
		return 0
	} else {
		return (size - free) / size
	}
}
