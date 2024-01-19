package main

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func drawDRO(th *material.Theme, gtx C, g *Grbl) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return drawGrblStatus(th, gtx, g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawCoords(th, gtx, g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawFeedSpeed(th, gtx, g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawGCodes(th, gtx, g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawBufferState(th, gtx, g)
		}),
		layout.Rigid(func(gtx C) D {
			return drawGrblModes(th, gtx, g)
		}),
	)
}

func drawGrblStatus(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H1(th, g.Status)
	return label.Layout(gtx)
}

func drawCoords(th *material.Theme, gtx C, g *Grbl) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return drawCoord(th, gtx, "X", g.Wpos.X)
		}),
		layout.Rigid(func(gtx C) D {
			return drawCoord(th, gtx, "Y", g.Wpos.Y)
		}),
		layout.Rigid(func(gtx C) D {
			return drawCoord(th, gtx, "Z", g.Wpos.Z)
		}),
	)
}

func drawCoord(th *material.Theme, gtx C, name string, value float64) D {
	label := material.H2(th, fmt.Sprintf("%s: %.03f", name, value))
	return label.Layout(gtx)
}

func drawFeedSpeed(th *material.Theme, gtx C, g *Grbl) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return drawCoord(th, gtx, "Feed", g.FeedRate)
		}),
		layout.Rigid(func(gtx C) D {
			return drawCoord(th, gtx, "Speed", g.SpindleSpeed)
		}),
	)
}

func drawGCodes(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H2(th, fmt.Sprintf("G0 G0 G0 G0"))
	return label.Layout(gtx)
}

func drawBufferState(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H2(th, fmt.Sprintf("Bf: %d/%d %d/%d", (g.PlannerSize-g.PlannerFree), g.PlannerSize, (g.SerialSize-g.SerialFree), g.SerialSize))
	return label.Layout(gtx)
}

func drawGrblModes(th *material.Theme, gtx C, g *Grbl) D {
	label := material.H2(th, fmt.Sprintf("[probe]"))
	return label.Layout(gtx)
}
