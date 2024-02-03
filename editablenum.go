package main

import (
	"image"

	"gioui.org/io/pointer"
	"gioui.org/op/clip"
	"gioui.org/unit"
)

type EditableNum struct {
	app      *App
	Label    string
	TextSize unit.Sp
	Callback func(float64)

	lastVal    float64
	numpop     *NumPop
	hovering   bool
	showEditor bool
}

func (e *EditableNum) Layout(gtx C, val float64) D {
	e.lastVal = val

	for _, gtxEvent := range gtx.Events(e) {
		switch gtxE := gtxEvent.(type) {
		case pointer.Event:
			if gtxE.Kind == pointer.Press {
				e.ShowEditor()
			} else if gtxE.Kind == pointer.Enter {
				e.hovering = true
			} else if gtxE.Kind == pointer.Leave {
				e.hovering = false
			}
		}
	}

	g := grey(0)
	if e.hovering {
		g = grey(16)
	}
	readout := Readout{th: e.app.th, decimalPlaces: 3, TextSize: e.TextSize, BackgroundColor: g}
	dims := readout.Layout(gtx, e.Label, val)

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	pointer.InputOp{
		Kinds: pointer.Press | pointer.Enter | pointer.Leave,
		Tag:   e,
	}.Add(gtx.Ops)

	if e.showEditor {
		if e.app.ShowingEditor() {
			gtx.Constraints.Max.X = dims.Size.X - 50
			e.numpop.Layout(gtx, image.Pt(50, dims.Size.Y))
		} else {
			e.HideEditor()
		}
	}

	return dims

}

func (e *EditableNum) ShowEditor() bool {
	if !e.app.ShowEditor() {
		return false
	}
	e.numpop = NewNumPop(e.app.th, e.lastVal, func(ok bool, val float64) {
		if ok {
			e.Callback(val)
		}
		e.HideEditor()
	})
	e.showEditor = true
	return true
}

func (e *EditableNum) HideEditor() {
	e.showEditor = false
	e.numpop = nil
	e.app.EditorHidden()
}
