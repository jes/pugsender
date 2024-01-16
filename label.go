package main

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
)

func drawLabel(th *material.Theme, gtx layout.Context) {
	label := material.H1(th, "Hello, world!")
	label.Layout(gtx)
}
