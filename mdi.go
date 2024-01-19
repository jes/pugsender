package main

import (
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type MDI struct {
	app    *App
	editor *widget.Editor
}

func NewMDI(app *App) *MDI {
	return &MDI{
		app: app,
		editor: &widget.Editor{
			SingleLine: true,
			Submit:     true,
		},
	}
}

func (m *MDI) Layout(gtx C) D {
	// handle MDI input
	for _, e := range m.editor.Events() {
		switch e.(type) {
		case widget.SubmitEvent:
			m.app.MDIInput(m.editor.Text())
			m.editor.SetText("")
		}
	}

	return widget.Border{Width: 1, CornerRadius: 2, Color: m.app.th.Palette.ContrastFg}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(5).Layout(gtx, func(gtx C) D {
			ed := material.Editor(m.app.th, m.editor, "")
			ed.Font = gofont.Collection()[6].Font
			return ed.Layout(gtx)
		})
	})

}
