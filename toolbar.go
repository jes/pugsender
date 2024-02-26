package main

import (
	"fmt"
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
	cgtx := gtx
	childCall := make([]op.CallOp, len(children))
	childDims := make([]D, len(children))

	// Lay out Rigid children.
	for i, child := range children {
		macro := op.Record(gtx.Ops)
		cgtx.Constraints = layout.Constraints{Min: image.Pt(0, cs.Min.Y), Max: image.Pt(cs.Max.X, cs.Max.Y)}
		dims := t.Inset.Layout(cgtx, child)
		c := macro.Stop()
		sz := layout.Horizontal.Convert(dims.Size).X
		size += sz
		childCall[i] = c
		childDims[i] = dims
	}

	maxHeight := cs.Min.Y
	for i := range children {
		if c := layout.Horizontal.Convert(childDims[i].Size).Y; c > maxHeight {
			maxHeight = c
		}
	}

	xOffset, yOffset := 0, 0
	totalWidth, totalHeight := 0, 0
	for i := range children {
		dims := childDims[i]
		width := layout.Horizontal.Convert(dims.Size).X
		if xOffset+width > cs.Max.X {
			fmt.Println(".")
			xOffset = 0
			// TODO: this makes each row equally tall, which is not necessarily the case
			// (each row only needs enough Y height to contain the items on that row),
			// but it's OK for now since we expect all buttons on a toolbar to have the
			// same height
			yOffset += maxHeight
		}
		if xOffset+width > totalWidth {
			totalWidth = xOffset + width
		}
		if yOffset+layout.Horizontal.Convert(dims.Size).Y > totalHeight {
			totalHeight = yOffset + layout.Horizontal.Convert(dims.Size).Y
		}

		pt := layout.Horizontal.Convert(image.Pt(xOffset, yOffset))
		trans := op.Offset(pt).Push(gtx.Ops)
		childCall[i].Add(gtx.Ops)
		trans.Pop()

		xOffset += width
	}

	sz := layout.Horizontal.Convert(image.Pt(totalWidth, totalHeight))
	return D{Size: cs.Constrain(sz)}
}
