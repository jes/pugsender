package main

import (
	"fmt"

	"gioui.org/widget/material"
)

func (a *App) LayoutStatusBar(gtx C) D {
	label := material.H4(a.th, fmt.Sprintf("mode=%s port=%s", a.mode, a.g.PortName))
	return label.Layout(gtx)
}
