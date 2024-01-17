package main

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func drawLabel(th *material.Theme, gtx layout.Context, g *Grbl) D {
	label := material.H1(th, fmt.Sprintf("%s: %.3f,%.3f,%.3f", g.Status, g.Wpos.X, g.Wpos.Y, g.Wpos.Z))
	return label.Layout(gtx)
}
