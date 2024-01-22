package main

import (
	"fmt"
	"image"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) LayoutToolpath(gtx C) D {
	a.path.Update(a.g.Mpos)
	a.path.crossHair = a.g.MposExt()
	a.path.axes.X = -a.g.Wco.X
	a.path.axes.Y = -a.g.Wco.Y

	borderColour := rgb(128, 128, 128)
	return Panel{Margin: 5, Width: 1, CornerRadius: 5, Color: borderColour}.Layout(gtx, func(gtx C) D {
		a.path.widthPx = gtx.Constraints.Min.X
		a.path.heightPx = gtx.Constraints.Min.Y
		if a.hovering {
			return layout.Stack{Alignment: layout.SE}.Layout(gtx,
				layout.Expanded(a.LayoutToolpathImage),
				layout.Stacked(func(gtx C) D {
					return material.H6(a.th, fmt.Sprintf("X%.03f Y%.03f", a.hoverPoint.X, a.hoverPoint.Y)).Layout(gtx)
				}),
			)
		} else {
			return a.LayoutToolpathImage(gtx)
		}
	})
}

func (a *App) LayoutToolpathImage(gtx C) D {
	for _, gtxEvent := range gtx.Events(a.path) {
		switch gtxE := gtxEvent.(type) {
		case pointer.Event:
			// get click point in work coordinates
			xMm, yMm := a.path.PxToMm(float64(gtxE.Position.X), float64(gtxE.Position.Y))
			xMm += a.g.Wco.X
			yMm += a.g.Wco.Y

			if gtxE.Kind == pointer.Scroll {
				a.path.pxPerMm *= 1.0 - float64(gtxE.Scroll.Y)/100.0
			} else if gtxE.Kind == pointer.Drag {
				if !a.dragging {
					a.dragging = true
					a.dragStart = gtxE.Position
					a.dragStartCentre = a.path.centre
				}
				origCentre := f32.Point{X: float32(a.dragStartCentre.X), Y: float32(a.dragStartCentre.Y)}
				newCentre := origCentre.Add((a.dragStart.Sub(gtxE.Position)).Div(float32(a.path.pxPerMm)))
				a.path.centre = V4d{X: float64(newCentre.X), Y: float64(newCentre.Y)}
			} else if gtxE.Kind == pointer.Release {
				if !a.dragging {
					if gtxE.Modifiers.Contain(key.ModCtrl) {
						// ctrl-click = jog
						a.jog.JogTo(xMm, yMm)
					} else if gtxE.Modifiers.Contain(key.ModShift) {
						// shift-click = set work offset
						pos := a.g.Wpos
						pos.X = xMm
						pos.Y = yMm
						a.g.SetWpos(pos)
					}
				}
				// TODO: right-click for context menu?
				a.dragging = false
			} else if gtxE.Kind == pointer.Move {
				a.hovering = true
			} else if gtxE.Kind == pointer.Leave {
				a.hovering = false
			}
			a.hoverPoint = V4d{X: xMm, Y: yMm}
		}
	}

	if a.g.Vel.Length() > 0.001 {
		// invalidate the frame if the velocity is non-zero,
		// because we need to redraw the plotted path
		// XXX: we should instead invalidate only when the rendering thread has a new plot to show
		a.w.Invalidate()
	}
	// TODO: render in a different thread
	a.path.Render()
	im := widget.Image{
		Src:   paint.NewImageOp(a.path.Image),
		Scale: 1.0 / gtx.Metric.PxPerDp,
	}

	dims := im.Layout(gtx)

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	pointer.InputOp{
		Kinds:        pointer.Scroll | pointer.Drag | pointer.Release | pointer.Move | pointer.Leave,
		Tag:          a.path,
		ScrollBounds: image.Rectangle{Min: image.Point{X: -50, Y: -50}, Max: image.Point{X: 50, Y: 50}},
	}.Add(gtx.Ops)

	return dims
}
