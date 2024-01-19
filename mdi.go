package main

import (
	"fmt"

	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/eventx"
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

	return widget.Border{Width: 1, CornerRadius: 2, Color: m.app.th.Palette.ContrastFg}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(5).Layout(gtx, func(gtx C) D {
			ed := material.Editor(m.app.th, m.editor, "")
			ed.Font = gofont.Collection()[6].Font

			// let the escape key defocus the input
			// https://github.com/gioui/gio/pull/38
			spy, spiedGtx := eventx.Enspy(gtx)
			dims := ed.Layout(spiedGtx)
			for _, group := range spy.AllEvents() {
				for _, event := range group.Items {
					switch e := event.(type) {
					case key.Event:
						if e.State == key.Press && e.Name == key.NameEscape {
							m.wantDefocus = true
						}
					}
				}
			}

			if m.wantDefocus {
				key.FocusOp{}.Add(gtx.Ops)
				m.wantDefocus = false
			}
			return dims
		})
	})
}
