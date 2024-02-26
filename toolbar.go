package main

import (
	"gioui.org/layout"
)

type Toolbar struct {
}

func (t Toolbar) Layout(gtx C, widgets ...layout.Widget) D {
	flexChilds := make([]layout.FlexChild, 0)

	for _, w := range widgets {
		w := w // capture loop variable for use in closure
		child := layout.Rigid(func(gtx C) D {
			return layout.UniformInset(5).Layout(gtx, w)
		})
		flexChilds = append(flexChilds, child)
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, flexChilds...)
}
