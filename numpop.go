package main

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Knetic/govaluate"
)

type NumPop struct {
	app     *App
	initVal float64
	cb      func(bool, float64)
	editor  *widget.Editor
}

func NewNumPop(app *App, initVal float64, cb func(bool, float64)) *NumPop {
	return &NumPop{
		app:     app,
		initVal: initVal,
		cb:      cb,
		editor: &widget.Editor{
			SingleLine: true,
			Submit:     true,
		},
	}
}

// don't use the returned value in calculating your dimensions unless
// you want the presence of the NumPop to change your returned dimensions
func (n *NumPop) Layout(gtx C, location image.Point) D {
	// XXX: use op.Record and op.Defer to defer the drawing of
	// the input popup until the end of the frame, so that the
	// popup is drawn on top of everything else
	macro := op.Record(gtx.Ops)

	// handle input
	for _, e := range n.editor.Events() {
		switch e.(type) {
		case widget.SubmitEvent:
			val, err := n.Value()
			n.cb(err == nil, val)
		default:
			fmt.Printf("[unhandled NumPop event] %#v\n", e)
		}
	}

	// dim the rest of the screen (XXX: why does Alpha have to be so high?)
	paint.Fill(gtx.Ops, rgba(0, 0, 0, 230))

	size := gtx.Constraints.Max

	offsetOp := op.Offset(location).Push(gtx.Ops)
	clipOp := clip.Rect{Max: size}.Push(gtx.Ops)
	gtx.Constraints.Max = size

	borderColour := grey(255)

	n.editor.Focus()

	dims := Panel{Width: 1, CornerRadius: 5, Color: grey(128), BackgroundColor: grey(32)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return material.H4(n.app.th, fmt.Sprintf("%.3f", n.initVal)).Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				// TODO: refactor this and the MDI editor into a common input box component
				return Panel{Margin: 5, Width: 1, CornerRadius: 2, Color: borderColour, BackgroundColor: grey(0), Padding: 5}.Layout(gtx, func(gtx C) D {
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

	clipOp.Pop()
	offsetOp.Pop()

	op.Defer(gtx.Ops, macro.Stop())

	return dims
}

func (n *NumPop) Value() (float64, error) {
	expr, err := govaluate.NewEvaluableExpression(n.editor.Text())
	if err != nil {
		// if the expression won't parse, try prefixing the initial value,
		// this makes inputs like "+1", "/2" work as expected
		expr, err = govaluate.NewEvaluableExpression(fmt.Sprintf("%f", n.initVal) + " " + n.editor.Text())
		if err != nil {
			return 0.0, err
		}
	}

	val, err := expr.Evaluate(nil)
	switch val := val.(type) {
	case float64:
		return val, nil
	default:
		return 0.0, err
	}
}
