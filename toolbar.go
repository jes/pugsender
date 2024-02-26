package main

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
)

type Toolbar struct {
	Alignment layout.Alignment
	Inset     layout.Inset
}

// Layout a list of widgets in a toolbar.
// Derived from layout.Flex.Layout()
func (t Toolbar) Layout(gtx C, children ...layout.Widget) D {
	size := 0
	cs := gtx.Constraints
	mainMax := cs.Max.X
	crossMin, crossMax := cs.Min.Y, cs.Max.Y
	remaining := mainMax
	cgtx := gtx
	childCall := make([]op.CallOp, len(children))
	childDims := make([]D, len(children))
	// Lay out Rigid children.
	for i, child := range children {
		macro := op.Record(gtx.Ops)
		cgtx.Constraints = layout.Constraints{Min: image.Pt(0, crossMin), Max: image.Pt(remaining, crossMax)}
		dims := t.Inset.Layout(cgtx, child)
		c := macro.Stop()
		sz := layout.Horizontal.Convert(dims.Size).X
		size += sz
		remaining -= sz
		if remaining < 0 {
			remaining = 0
		}
		childCall[i] = c
		childDims[i] = dims
	}
	// fraction is the rounding error from a Flex weighting.
	maxCross := crossMin
	var maxBaseline int
	for i := range children {
		if c := layout.Horizontal.Convert(childDims[i].Size).Y; c > maxCross {
			maxCross = c
		}
		if b := childDims[i].Size.Y - childDims[i].Baseline; b > maxBaseline {
			maxBaseline = b
		}
	}
	var mainSize int
	for i := range children {
		dims := childDims[i]
		b := dims.Size.Y - dims.Baseline
		var cross int
		switch t.Alignment {
		case layout.End:
			cross = maxCross - layout.Horizontal.Convert(dims.Size).Y
		case layout.Middle:
			cross = (maxCross - layout.Horizontal.Convert(dims.Size).Y) / 2
		case layout.Baseline:
			cross = maxBaseline - b
		}
		pt := layout.Horizontal.Convert(image.Pt(mainSize, cross))
		trans := op.Offset(pt).Push(gtx.Ops)
		childCall[i].Add(gtx.Ops)
		trans.Pop()
		mainSize += layout.Horizontal.Convert(dims.Size).X
	}
	sz := layout.Horizontal.Convert(image.Pt(mainSize, maxCross))
	sz = cs.Constrain(sz)
	return D{Size: sz, Baseline: sz.Y - maxBaseline}
}
