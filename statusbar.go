package main

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func (a *App) LayoutStatusBar(gtx C) D {
	// TODO: stop hard-coding colours everywhere, make the app have a theme that is more expansive than material.Theme
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 32, G: 32, B: 32, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
		return D{Size: gtx.Constraints.Min}
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return drawImage(gtx, a.img)
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.mode.String()).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.g.PortName).Layout),
		)
	})
}
