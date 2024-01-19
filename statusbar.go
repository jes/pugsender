package main

import (
	"fmt"

	"gioui.org/widget/material"
)

func drawStatusBar(th *material.Theme, gtx C, a *App) D {
	label := material.H4(th, fmt.Sprintf("mode=%s", a.mode))
	return label.Layout(gtx)
}
