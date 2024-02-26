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

type ToolpathView struct {
	app             *App
	path            *Path
	dragStart       f32.Point
	dragStartCentre V4d
	dragPoint       V4d
	dragging        bool
	hovering        bool
	hoverPoint      V4d
	rendering       bool
	imageOp         paint.ImageOp
}

func NewToolpathView(app *App) *ToolpathView {
	tp := &ToolpathView{}
	tp.app = app
	tp.path = NewPath()
	tp.path.showCrossHair = true
	tp.path.showAxes = true
	tp.path.showGridLines = true
	tp.path.Render()
	tp.imageOp = paint.NewImageOp(tp.path.Image)
	return tp
}

func (tp *ToolpathView) Layout(gtx C) D {
	tp.path.Update(tp.app.gs.Mpos)
	tp.path.crossHair = tp.app.gs.Mpos
	tp.path.axes.X = tp.app.gs.Wco.X
	tp.path.axes.Y = tp.app.gs.Wco.Y

	// render the toolpath in a different goroutine so as not to
	// block the main UI
	if !tp.rendering {
		tp.rendering = true
		go func() {
			if tp.path.Render() {
				tp.imageOp = paint.NewImageOp(tp.path.Image)
				tp.app.w.Invalidate()
			}
			tp.rendering = false
		}()
	}

	borderColour := rgb(128, 128, 128)
	return Panel{Margin: layout.UniformInset(5), Width: 1, CornerRadius: 5, Color: borderColour}.Layout(gtx, func(gtx C) D {
		tp.path.widthPx = gtx.Constraints.Min.X
		tp.path.heightPx = gtx.Constraints.Min.Y
		if tp.hovering {
			return layout.Stack{Alignment: layout.NE}.Layout(gtx,
				layout.Expanded(func(gtx C) D {
					return layout.Stack{Alignment: layout.SE}.Layout(gtx,
						layout.Expanded(tp.LayoutImage),
						layout.Stacked(func(gtx C) D {
							return material.H6(tp.app.th, fmt.Sprintf("X%.03f Y%.03f", tp.hoverPoint.X, tp.hoverPoint.Y)).Layout(gtx)
						}),
					)
				}),
				layout.Stacked(func(gtx C) D {
					return material.H6(tp.app.th, " Ctrl-click = jog\nShift-click = set WCO").Layout(gtx)
				}),
			)
		} else {
			return tp.LayoutImage(gtx)
		}
	})
}

func (tp *ToolpathView) LayoutImage(gtx C) D {
	for _, gtxEvent := range gtx.Events(tp.path) {
		switch gtxE := gtxEvent.(type) {
		case pointer.Event:
			// get click point in work coordinates
			xMm, yMm := tp.path.PxToMm(float64(gtxE.Position.X), float64(gtxE.Position.Y))
			xMm -= tp.app.gs.Wco.X
			yMm -= tp.app.gs.Wco.Y

			if gtxE.Kind == pointer.Scroll {
				tp.path.pxPerMm *= 1.0 - float64(gtxE.Scroll.Y)/100.0
			} else if gtxE.Kind == pointer.Drag {
				if !tp.dragging {
					tp.dragging = true
					tp.dragStart = gtxE.Position
					tp.dragStartCentre = tp.path.centre
					tp.dragPoint = V4d{X: xMm, Y: yMm}
				}
				origCentre := f32.Point{X: float32(tp.dragStartCentre.X), Y: float32(tp.dragStartCentre.Y)}
				newCentre := origCentre.Add((tp.dragStart.Sub(gtxE.Position)).Div(float32(tp.path.pxPerMm)))
				tp.path.centre = V4d{X: float64(newCentre.X), Y: float64(newCentre.Y)}
			} else if gtxE.Kind == pointer.Release {
				if !tp.dragging {
					if gtxE.Modifiers.Contain(key.ModCtrl) {
						// ctrl-click = jog
						tp.app.jog.JogTo(xMm, yMm)
					} else if gtxE.Modifiers.Contain(key.ModShift) {
						// shift-click = set work offset
						wpos := tp.app.gs.Wpos
						wpos.X = xMm
						wpos.Y = yMm
						tp.app.SetWpos(wpos)
					}
				}
				// TODO: right-click for context menu?
				tp.dragging = false
			} else if gtxE.Kind == pointer.Move {
				tp.hovering = true
			} else if gtxE.Kind == pointer.Leave {
				tp.hovering = false
			}
			if tp.dragging {
				tp.hoverPoint = tp.dragPoint
			} else {
				tp.hoverPoint = V4d{X: xMm, Y: yMm}
			}
		}
	}

	if tp.app.gs.Vel.Length() > 0.001 {
		// invalidate the frame if the velocity is non-zero,
		// because we need to redraw the plotted path
		// XXX: we should instead invalidate only when the rendering thread has a new plot to show
		tp.app.w.Invalidate()
	}
	im := widget.Image{
		Src:   tp.imageOp,
		Scale: 1.0 / gtx.Metric.PxPerDp,
	}

	dims := im.Layout(gtx)

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	pointer.InputOp{
		Kinds:        pointer.Scroll | pointer.Drag | pointer.Release | pointer.Move | pointer.Leave,
		Tag:          tp.path,
		ScrollBounds: image.Rectangle{Min: image.Point{X: -50, Y: -50}, Max: image.Point{X: 50, Y: 50}},
	}.Add(gtx.Ops)

	return dims
}
