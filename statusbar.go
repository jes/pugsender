package main

import "gioui.org/widget/material"

func drawStatusBar(th *material.Theme, gtx C, a *App) D {
	label := material.H4(th, "[status bar here]")
	return label.Layout(gtx)
}
