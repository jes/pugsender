package main

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Label struct {
	app  *App
	text string
}

func (l Label) Layout(gtx C) D {
	label := material.H4(l.app.th, l.text)
	borderColor := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	return layout.UniformInset(4).Layout(gtx, func(gtx C) D {
		return widget.Border{Width: 1, CornerRadius: 2, Color: borderColor}.Layout(gtx, func(gtx C) D {
			return layout.UniformInset(4).Layout(gtx, label.Layout)
		})
	})

}
