package main

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type MDI struct {
	app         *App
	editor      *widget.Editor
	wantDefocus bool
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

func (m *MDI) Defocus() {
	m.wantDefocus = true
	m.app.w.Invalidate()
}

func (m *MDI) Layout(gtx C) D {
	// handle MDI input
	for _, e := range m.editor.Events() {
		switch e.(type) {
		case widget.SubmitEvent:
			m.app.MDIInput(m.editor.Text())
			m.editor.SetText("")
		default:
			fmt.Printf("%#v\n", e)
		}
	}

	m.editor.ReadOnly = (m.app.mode == ModeConnect)

	borderColour := grey(255)
	if m.editor.ReadOnly {
		borderColour = grey(100)
	} else if !m.editor.Focused() {
		borderColour = grey(128)
	}

	return Panel{Margin: 5, Width: 1, CornerRadius: 2, Color: borderColour, Padding: 5}.Layout(gtx, func(gtx C) D {
		ed := material.Editor(m.app.th, m.editor, "")
		if m.wantDefocus {
			key.FocusOp{}.Add(gtx.Ops)
			m.wantDefocus = false
		}
		return ed.Layout(gtx)
	})
}
