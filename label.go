package main

import (
	"gioui.org/widget/material"
)

type Label struct {
	app  *App
	text string
}

func (l Label) Layout(gtx C) D {
	label := material.H4(l.app.th, l.text)
	borderColour := grey(128)

	return Panel{Width: 1, CornerRadius: 2, Color: borderColour, Margin: 4, Padding: 4}.Layout(gtx, label.Layout)

}
