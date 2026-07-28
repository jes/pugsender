package main

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type MDI struct {
	app    *App
	editor *widget.Editor

	wantDefocus     bool
	defocusOnSubmit bool

	history      []string
	historyIndex int
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
	m.defocusOnSubmit = false
	m.wantDefocus = true
	m.app.w.Invalidate()
	m.historyIndex = len(m.history)
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

	if m.editor.Focused() {
		// handle arrow keys
		for _, gtxEvent := range gtx.Events(m) {
			switch gtxE := gtxEvent.(type) {
			case key.Event:
				if gtxE.State == key.Press {
					if gtxE.Name == key.NameUpArrow {
						if m.historyIndex > 0 {
							m.historyIndex -= 1
						}
					} else if gtxE.Name == key.NameDownArrow {
						if m.historyIndex < len(m.history) {
							m.historyIndex += 1
						}
					}

					fmt.Printf("idx=%d\n", m.historyIndex)

					// if we've scrolled to a valid history point, set text
					if m.historyIndex >= 0 && m.historyIndex < len(m.history) {
						m.editor.SetText(m.history[m.historyIndex])
						m.defocusOnSubmit = false
					} else {
						m.editor.SetText("")
					}
				}
			}
		}

		key.InputOp{
			Keys: key.Set(key.NameUpArrow + "|" + key.NameDownArrow),
			Tag:  m,
		}.Add(gtx.Ops)
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
