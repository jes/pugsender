package main

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) LayoutDRO(gtx C) D {
	return layout.UniformInset(5).Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.LayoutGrblStatus),
			layout.Rigid(layout.Spacer{Height: 5}.Layout),
			layout.Rigid(a.LayoutDROCoords),
			layout.Rigid(layout.Spacer{Height: 5}.Layout),
			layout.Rigid(a.LayoutFeedSpeed),
			layout.Rigid(layout.Spacer{Height: 5}.Layout),
			layout.Rigid(a.LayoutGCodes),
			layout.Rigid(func(gtx C) D {
				return drawGrblModes(a.th, gtx, a.gs)
			}),
			layout.Rigid(a.LayoutJogState),
		)
	})
}

func (a *App) LayoutGrblStatus(gtx C) D {
	status := a.gs.Status
	if status == "Hold:0" {
		// Hold complete. Ready to resume.
		status = "Hold"
	} else if status == "Hold:1" {
		// Hold in-progress. Reset will throw an alarm.
		status = "Hold..."
	}

	label := material.H4(a.th, strings.ToUpper(status))
	label.Alignment = text.Middle
	borderColour := grey(128)
	return widget.Border{Width: 1, CornerRadius: 2, Color: borderColour}.Layout(gtx, func(gtx C) D {
		return LayoutColour(gtx, grey(32), func(gtx C) D {
			return layout.UniformInset(5).Layout(gtx, func(gtx C) D {
				return label.Layout(gtx)
			})
		})
	})
}

func (a *App) LayoutDROCoords(gtx C) D {
	if a.gs.Vel.Length() > 0.001 {
		// invalidate the frame if the velocity is non-zero,
		// because we need to redraw the extrapolated coordinates
		a.w.Invalidate()
	}

	return Panel{Width: 1, Color: grey(128), CornerRadius: 5, Padding: layout.UniformInset(5), BackgroundColor: grey(32)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return a.xDro.Layout(gtx, a.g.WposExt().X)
			}),
			layout.Rigid(func(gtx C) D {
				return a.yDro.Layout(gtx, a.g.WposExt().Y)
			}),
			layout.Rigid(func(gtx C) D {
				return a.zDro.Layout(gtx, a.g.WposExt().Z)
			}),
			// TODO: optional 4th axis?
		)
	})
}

func (a *App) LayoutFeedSpeed(gtx C) D {
	readout := Readout{th: a.th, TextSize: material.H5(a.th, "").TextSize, BackgroundColor: grey(0)}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return readout.Layout(gtx, " Feed", a.gs.FeedRate)
		}),
		layout.Rigid(func(gtx C) D {
			return readout.Layout(gtx, "Speed", a.gs.SpindleSpeed)
		}),
	)
}

func (a *App) LayoutGCodes(gtx C) D {
	label := material.H6(a.th, fmt.Sprintf(a.gs.GCodes))
	return label.Layout(gtx)
}

func (a *App) LayoutJogState(gtx C) D {
	return Panel{Width: 1, Color: grey(128), CornerRadius: 5, Padding: layout.UniformInset(5), BackgroundColor: grey(32)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return material.H6(a.th, "Jog").Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				return a.jogIncEdit.Layout(gtx, a.jog.Increment)
			}),
			layout.Rigid(func(gtx C) D {
				return a.jogFeedEdit.Layout(gtx, a.jog.FeedRate)
			}),
			layout.Rigid(func(gtx C) D {
				return a.jogRapidFeedEdit.Layout(gtx, a.jog.RapidFeedRate)
			}),
		)
	})

}

func drawGrblModes(th *material.Theme, gtx C, gs GrblStatus) D {
	probeStr := ""
	if gs.Probe {
		probeStr = "[probe]"
	}
	label := material.H5(th, probeStr)
	return label.Layout(gtx)
}
