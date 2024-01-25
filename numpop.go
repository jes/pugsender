package main

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/soniah/evaler"
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
			val, ok := n.Value()
			n.cb(ok, val)
		default:
			fmt.Printf("[unhandled NumPop event] %#v\n", e)
		}
	}

	// dim the rest of the screen (XXX: why does Alpha have to be so high?)
	paint.Fill(gtx.Ops, rgba(0, 0, 0, 230))

	offsetOp := op.Offset(location).Push(gtx.Ops)

	borderColour := grey(255)

	n.editor.Focus()

	dims := Panel{Width: 1, CornerRadius: 5, Color: grey(128), BackgroundColor: grey(32), Padding: layout.UniformInset(4)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				v := n.initVal
				if val, ok := n.Value(); ok {
					v = val
				}
				label := material.H4(n.app.th, fmt.Sprintf("%.3f ", v))
				label.Alignment = text.End
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				// TODO: refactor this and the MDI editor into a common input box component
				return Panel{Margin: layout.UniformInset(5), Width: 1, CornerRadius: 2, Color: borderColour, BackgroundColor: grey(0), Padding: layout.UniformInset(5)}.Layout(gtx, func(gtx C) D {
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

	offsetOp.Pop()

	op.Defer(gtx.Ops, macro.Stop())

	return dims
}

// https://stackoverflow.com/a/65484336
func (n *NumPop) Value() (float64, bool) {
	// first try the expression as-is
	val, ok := eval(n.editor.Text())
	if ok {
		return val, true
	}

	// failing that, try prepending the existing value (for "/2" etc.)
	return eval(fmt.Sprintf("%f %s", n.initVal, n.editor.Text()))
}

func eval(line string) (float64, bool) {
	valrat, err := evaler.Eval(line)
	if err == nil {
		val, _ := valrat.Float64()
		return val, true
	} else {
		return 0.0, false
	}
}
