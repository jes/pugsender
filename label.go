package main

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
)

func drawLabel(th *material.Theme, gtx layout.Context, g *Grbl) D {
	label := material.H1(th, g.Status)
	return label.Layout(gtx)
}
