package main

import (
	"fmt"
	"image"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type NumPop struct {
	app      *App
	initVal  float64
	location image.Point
	cb       func(bool, float64)
	editor   *widget.Editor
}

func NewNumPop(app *App, initVal float64, location image.Point, cb func(bool, float64)) *NumPop {
	return &NumPop{
		app:      app,
		initVal:  initVal,
		location: location,
		cb:       cb,
		editor: &widget.Editor{
			SingleLine: true,
			Submit:     true,
		},
	}
}

func (n *NumPop) Layout(gtx C) D {
	// handle input
	for _, e := range n.editor.Events() {
		switch e.(type) {
		case widget.SubmitEvent:
			val, err := strconv.ParseFloat(n.editor.Text(), 64)
			n.cb(err == nil, val)
		default:
			fmt.Printf("[unhandled NumPop event] %#v\n", e)
		}
	}

	size := image.Pt(500, 300)

	defer op.Offset(n.location).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	gtx.Constraints.Max = size

	borderColour := grey(255)

	n.editor.Focus()

	return Panel{Width: 1, CornerRadius: 5, Color: grey(128), BackgroundColor: grey(32)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return material.H4(n.app.th, fmt.Sprintf("%.3f", n.initVal)).Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				// TODO: refactor this and the MDI editor into a common input box component
				return Panel{Margin: 5, Width: 1, CornerRadius: 2, Color: borderColour, Padding: 5}.Layout(gtx, func(gtx C) D {
					ed := material.Editor(n.app.th, n.editor, "")
					/*TODO: if n.wantDefocus {
						key.FocusOp{}.Add(gtx.Ops)
						n.wantDefocus = false
					}*/
					return ed.Layout(gtx)
				})
			}),
		)
	})
}
