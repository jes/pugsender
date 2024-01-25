package main

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Panel struct {
	Width           unit.Dp     // of border
	CornerRadius    unit.Dp     // of border
	Color           color.NRGBA // of border
	BackgroundColor color.NRGBA
	Margin          layout.Inset // outside the border
	Padding         layout.Inset // inside the border
}

type Label struct {
	th   *material.Theme
	text string
}

type Readout struct {
	th              *material.Theme
	TextSize        unit.Sp
	decimalPlaces   int
	BackgroundColor color.NRGBA
}

// draw widget with the given background colour
func LayoutColour(gtx C, col color.NRGBA, widget layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, col, clip.Rect{Max: gtx.Constraints.Min}.Op())
		return D{Size: gtx.Constraints.Min}
	}, widget)
}

func LayoutColourRR(gtx C, col color.NRGBA, rr int, widget layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 5).Op(gtx.Ops))
		return D{Size: gtx.Constraints.Min}
	}, widget)

}

// based on material.ProgressBar
func LayoutProgressBar(gtx C, progress float64, th *material.Theme, text string) D {
	shader := func(width int, color color.NRGBA) layout.Dimensions {
		d := image.Point{X: width, Y: gtx.Dp(unit.Dp(th.TextSize - 1))}

		defer clip.Rect(image.Rectangle{Max: image.Pt(width, d.Y)}).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: color}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		return layout.Dimensions{Size: d}
	}

	colour1 := grey(128)
	colour2 := grey(255)

	progressBarWidth := int(100 * th.TextSize / 16.0)
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
		layout.Expanded(material.Body1(th, text).Layout),
	)
}

func (p Panel) Layout(gtx C, w layout.Widget) D {
	if p.BackgroundColor.A > 0 {
		return p.Margin.Layout(gtx, func(gtx C) D {
			return widget.Border{Width: p.Width, CornerRadius: p.CornerRadius, Color: p.Color}.Layout(gtx, func(gtx C) D {
				return LayoutColourRR(gtx, p.BackgroundColor, gtx.Metric.Dp(p.CornerRadius), func(gtx C) D {
					return p.Padding.Layout(gtx, w)
				})
			})
		})
	} else {
		return p.Margin.Layout(gtx, func(gtx C) D {
			return widget.Border{Width: p.Width, CornerRadius: p.CornerRadius, Color: p.Color}.Layout(gtx, func(gtx C) D {
				return p.Padding.Layout(gtx, w)
			})
		})
	}
}

func (l Label) Layout(gtx C) D {
	label := material.H5(l.th, l.text)
	borderColour := grey(128)

	return Panel{Width: 1, CornerRadius: 2, Color: borderColour, Margin: layout.UniformInset(4), Padding: layout.UniformInset(4)}.Layout(gtx, label.Layout)

}

func (r Readout) Layout(gtx C, name string, value float64) D {
	nameLabel := material.Label(r.th, r.TextSize, name)
	valueLabel := material.Label(r.th, r.TextSize, fmt.Sprintf("%.*f ", r.decimalPlaces, value))
	valueLabel.Alignment = text.End

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(layout.Spacer{Width: 5}.Layout),
		layout.Rigid(nameLabel.Layout),
		layout.Rigid(layout.Spacer{Width: 10}.Layout),
		layout.Flexed(1, func(gtx C) D {
			return Panel{Width: 1, Color: grey(128), CornerRadius: 5, BackgroundColor: r.BackgroundColor, Margin: layout.UniformInset(2)}.Layout(gtx, valueLabel.Layout)
		}),
	)

}

func rgb(r uint8, g uint8, b uint8) color.NRGBA {
	return rgba(r, g, b, 255)
}

func rgba(r uint8, g uint8, b uint8, a uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

func grey(v uint8) color.NRGBA {
	return rgb(v, v, v)
}
