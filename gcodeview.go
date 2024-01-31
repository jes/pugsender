package main

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var list *widget.List
var scrolledTo int

func (a *App) LayoutGcode(gtx C) D {
	if list == nil {
		var l widget.List
		l.Axis = layout.Vertical
		list = &l
	}

	// auto-scroll the view whenever a new line is sent
	scrollTarget := a.nextLine - 15
	if scrolledTo != scrollTarget {
		list.ScrollTo(scrollTarget)
		scrolledTo = scrollTarget
	}

	return Panel{Width: 1, CornerRadius: 5, Color: grey(128), BackgroundColor: grey(16), Margin: layout.UniformInset(5), Padding: layout.UniformInset(5)}.Layout(gtx, func(gtx C) D {
		return material.List(a.th, list).Layout(gtx, len(a.gcode), func(gtx C, i int) D {
			// TODO: colour differently based on all states?
			// * not sent yet
			// * waiting in serial buffer
			// * waiting in planner buffer
			// * currently executing
			// * completed
			if i < a.nextLine {
				return LayoutColour(gtx, grey(32), material.Body1(a.th, a.gcode[i]).Layout)
			} else {
				return material.Body1(a.th, a.gcode[i]).Layout(gtx)
			}
		})
	})
}
