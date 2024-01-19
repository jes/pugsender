package main

import (
	"fmt"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func drawMDI(th *material.Theme, gtx C, editor *widget.Editor, g *Grbl) D {
	// handle MDI input
	for _, e := range editor.Events() {
		switch e.(type) {
		case widget.SubmitEvent:
			g.Write([]byte(editor.Text() + "\n"))
			fmt.Printf(" > [%s]\n", editor.Text())
			editor.SetText("")
		}
	}

	return widget.Border{Width: 1, CornerRadius: 2, Color: th.Palette.ContrastFg}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(5).Layout(gtx, func(gtx C) D {
			ed := material.Editor(th, editor, "")
			ed.Font = gofont.Collection()[6].Font
			return ed.Layout(gtx)
		})
	})

}
