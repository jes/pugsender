package main

import (
	"fmt"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func (a *App) LayoutDRO(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return drawGrblStatus(a.th, gtx, a.g)
		}),
		layout.Rigid(a.LayoutDROCoords),
		layout.Rigid(a.LayoutFeedSpeed),
		layout.Rigid(func(gtx C) D {
			return drawGCodes(a.th, gtx, a.g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawGrblModes(a.th, gtx, a.g)
		}),
	)
}

func drawGrblStatus(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H4(th, g.Status)
	return label.Layout(gtx)
}

func (a *App) LayoutDROCoords(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return a.LayoutDROCoord(gtx, "X", a.g.Wpos.X, a.g.Vel.X, a.g.UpdateTime)
		}),
		layout.Rigid(func(gtx C) D {
			return a.LayoutDROCoord(gtx, "Y", a.g.Wpos.Y, a.g.Vel.Y, a.g.UpdateTime)
		}),
		layout.Rigid(func(gtx C) D {
			return a.LayoutDROCoord(gtx, "Z", a.g.Wpos.Z, a.g.Vel.Z, a.g.UpdateTime)
		}),
	)
}

func (a *App) LayoutDROCoord(gtx C, name string, value float64, vel float64, lastUpdate time.Time) D {
	dt := time.Now().Sub(lastUpdate)
	value = value + vel*dt.Minutes()
	if math.Abs(vel) > 0.001 {
		a.w.Invalidate()
	}
	label := material.H4(a.th, fmt.Sprintf("%s: %.03f", name, value))
	return label.Layout(gtx)
}

func (a *App) LayoutFeedSpeed(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			label := material.H4(a.th, fmt.Sprintf("Feed: %.0f", a.g.FeedRate))
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			label := material.H4(a.th, fmt.Sprintf("Speed: %.0f", a.g.SpindleSpeed))
			return label.Layout(gtx)
		}),
	)
}

func drawGCodes(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H4(th, fmt.Sprintf(g.GCodes))
	return label.Layout(gtx)
}

func drawGrblModes(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H4(th, fmt.Sprintf("[probe]"))
	return label.Layout(gtx)
}
