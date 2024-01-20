package main

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func (a *App) LayoutStatusBar(gtx C) D {
	// TODO: stop hard-coding colours everywhere, make the app have a theme that is more expansive than material.Theme
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 32, G: 32, B: 32, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
		return D{Size: gtx.Constraints.Min}
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx C) D {
				return drawImage(gtx, a.img)
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.mode.String()).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.Label(a.g.PortName).Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(a.LayoutBufferState),
		)
	})
}

func (a *App) LayoutBufferState(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			// planner buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.g.PlannerSize), float64(a.g.PlannerFree)))
		}),
		layout.Rigid(layout.Spacer{Height: 4}.Layout),
		layout.Rigid(func(gtx C) D {
			// serial buffer
			return LayoutProgressBar(gtx, utilisation(float64(a.g.SerialSize), float64(a.g.SerialFree)))
		}),
	)
}

// based on material.ProgressBar
func LayoutProgressBar(gtx C, progress float64) D {
	shader := func(width int, color color.NRGBA) layout.Dimensions {
		d := image.Point{X: width, Y: gtx.Dp(18)}

		defer clip.Rect(image.Rectangle{Max: image.Pt(width, d.Y)}).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: color}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		return layout.Dimensions{Size: d}
	}

	colour1 := color.NRGBA{R: 64, G: 128, B: 64, A: 255}
	colour2 := color.NRGBA{R: 64, G: 255, B: 64, A: 255}

	progressBarWidth := 100
	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return shader(progressBarWidth, colour1)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			fillWidth := int(float64(progressBarWidth) * clamp1(progress))
			fillColor := colour2
			return shader(fillWidth, fillColor)
		}),
	)
}

// clamp1 limits v to range [0..1].
func clamp1(v float64) float64 {
	if v >= 1 {
		return 1
	} else if v <= 0 {
		return 0
	} else {
		return v
	}
}

func utilisation(size float64, free float64) float64 {
	if size == 0 {
		return 0
	} else {
		return (size - free) / size
	}
}
