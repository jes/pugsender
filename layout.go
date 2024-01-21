package main

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Panel struct {
	Width           unit.Dp     // of border
	CornerRadius    unit.Dp     // of border
	Color           color.NRGBA // of border
	BackgroundColor color.NRGBA
	Margin          unit.Dp // outside the border
	Padding         unit.Dp // inside the border
}

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

	colour1 := grey(128)
	colour2 := grey(255)

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

func (p Panel) Layout(gtx C, w layout.Widget) D {
	if p.BackgroundColor.A > 0 {
		return layout.UniformInset(p.Margin).Layout(gtx, func(gtx C) D {
			return widget.Border{Width: p.Width, CornerRadius: p.CornerRadius, Color: p.Color}.Layout(gtx, func(gtx C) D {
				return LayoutColour(gtx, p.BackgroundColor, func(gtx C) D {
					return layout.UniformInset(p.Padding).Layout(gtx, w)
				})
			})
		})
	} else {
		return layout.UniformInset(p.Margin).Layout(gtx, func(gtx C) D {
			return widget.Border{Width: p.Width, CornerRadius: p.CornerRadius, Color: p.Color}.Layout(gtx, func(gtx C) D {
				return layout.UniformInset(p.Padding).Layout(gtx, w)
			})
		})
	}
}

func rgb(r uint8, g uint8, b uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func rgba(r uint8, g uint8, b uint8, a uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

func grey(v uint8) color.NRGBA {
	return rgb(v, v, v)
}
