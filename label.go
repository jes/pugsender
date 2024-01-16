package main

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
)

func drawLabel(th *material.Theme, gtx layout.Context) D {
	label := material.H1(th, "Hello, world!")
	return label.Layout(gtx)
}
