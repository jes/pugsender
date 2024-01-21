package main

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
)

// draw widget with the given background colour
func LayoutColour(gtx C, col color.NRGBA, widget layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, col, clip.Rect{Max: gtx.Constraints.Min}.Op())
		return D{Size: gtx.Constraints.Min}
	}, widget)
}

// based on material.ProgressBar
func LayoutProgressBar(gtx C, progress float64, th *material.Theme, text string) D {
	shader := func(width int, color color.NRGBA) layout.Dimensions {
		d := image.Point{X: width, Y: gtx.Dp(18)}

		defer clip.Rect(image.Rectangle{Max: image.Pt(width, d.Y)}).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: color}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		return layout.Dimensions{Size: d}
	}

	colour1 := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	colour2 := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	progressBarWidth := 100
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return layout.Stack{Alignment: layout.W}.Layout(gtx,
				layout.Stacked(func(gtx C) D {
					return shader(progressBarWidth, colour1)
				}),
				layout.Stacked(func(gtx C) D {
					fillWidth := int(float64(progressBarWidth) * clamp1(progress))
					fillColor := colour2
					return shader(fillWidth, fillColor)
				}),
			)
		}),
		layout.Expanded(material.H6(th, text).Layout),
	)
}
