package main

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type MDI struct {
	app         *App
	editor      *widget.Editor
	wantDefocus bool
	history     []string // TODO: save the history to disk?
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
			m.history = append(m.history, m.editor.Text())
			m.app.MDIInput(m.editor.Text())
			m.editor.SetText("")
		default:
			fmt.Printf("[unhandled MDI event] %#v\n", e)
		}
	}

	borderColour := grey(255)
	if !m.editor.Focused() {
		borderColour = grey(128)
	}

	ed := material.Editor(m.app.th, m.editor, "")
	if m.wantDefocus {
		key.FocusOp{}.Add(gtx.Ops)
		m.wantDefocus = false
	}

	label := material.Label(m.app.th, m.app.th.TextSize, "MDI>")

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return Panel{Margin: layout.Inset{Top: 5, Bottom: 5}, Width: 1, CornerRadius: 2, Padding: layout.Inset{Top: 5, Bottom: 5}}.Layout(gtx, label.Layout)
		}),
		layout.Flexed(1, func(gtx C) D {
			return Panel{Margin: layout.UniformInset(5), Width: 1, CornerRadius: 2, Color: borderColour, Padding: layout.UniformInset(5)}.Layout(gtx, ed.Layout)
		}),
	)
}
