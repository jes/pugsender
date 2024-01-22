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
				return drawImage(gtx, a.img)
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.mode.String()).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.g.PortName).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.LayoutBufferState),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(fmt.Sprintf("Pn:%s", a.g.Pn)).Layout),
		)
	})
}

func (a *App) LayoutBufferState(gtx C) D {
	th := material.NewTheme()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			// planner buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.g.PlannerSize), float64(a.g.PlannerFree)), th, "PLAN")
		}),
		layout.Rigid(func(gtx C) D {
			// serial buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.g.SerialSize), float64(a.g.SerialFree)), th, "SER")
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
