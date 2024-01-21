package main

import (
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d/draw2dimg"
)

type Path struct {
	positions     []V4d
	showAxes      bool
	showCrossHair bool
	crossHair     V4d
	pxPerMm       float64
	centre        V4d // what coordinate is in the centre?
	widthPx       int
	heightPx      int
}

func NewPath() *Path {
	return &Path{pxPerMm: 10}
}

func (p *Path) Update(pos V4d) {
	// TODO: if this point lies on a straight line through
	// the last 2 points, simplify the path by replacing the
	// last point instead of appending
	p.positions = append(p.positions, pos)
}

// TODO: store the image we made, and next time we render
// with the same parameters, only render the new points on
// top, instead of starting from scratch every time
func (p *Path) Render() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, p.widthPx, p.heightPx))
	gc := draw2dimg.NewGraphicContext(img)

	if p.showAxes {
		centrex, centrey := p.MmToPx(0, 0)

		gc.SetStrokeColor(rgb(64, 0, 0))
		gc.MoveTo(centrex, 0)
		gc.LineTo(centrex, float64(p.heightPx))
		gc.Stroke()

		gc.SetStrokeColor(rgb(0, 64, 0))
		gc.MoveTo(0, centrey)
		gc.LineTo(float64(p.widthPx), centrey)
		gc.Stroke()
	}

	l := len(p.positions)
	if l > 0 {
		gc.SetStrokeColor(color.White)
		gc.MoveTo(p.MmToPx(p.positions[0].X, p.positions[0].Y))
		for _, pos := range p.positions {
			gc.LineTo(p.MmToPx(pos.X, pos.Y))
		}
		gc.Stroke()
	}

	if p.showCrossHair {
		gc.SetStrokeColor(grey(128))
		x, y := p.MmToPx(p.centre.X, p.centre.Y)
		p.DrawCrossHair(gc, x, y, 12)
	}

	return img
}

func (p *Path) DrawCrossHair(gc *draw2dimg.GraphicContext, x, y, r float64) {
	gc.MoveTo(x, y+r)
	for angle := 0.0; angle <= 360; angle += 5 {
		dx := r * math.Sin(angle*math.Pi/180.0)
		dy := r * math.Cos(angle*math.Pi/180.0)
		gc.LineTo(x+dx, y+dy)
	}
	gc.Stroke()

	gc.MoveTo(x, y+r*2)
	gc.LineTo(x, y-r*2)
	gc.Stroke()

	gc.MoveTo(x+r*2, y)
	gc.LineTo(x-r*2, y)
	gc.Stroke()
}

func (p *Path) MmToPx(x, y float64) (float64, float64) {
	halfWidth := float64(p.widthPx / 2)
	halfHeight := float64(p.heightPx / 2)
	return p.pxPerMm*(x-p.centre.X) + halfWidth, p.pxPerMm*(-y-p.centre.Y) + halfHeight
}

func (p *Path) PxToMm(x, y float64) (float64, float64) {
	halfWidth := float64(p.widthPx / 2)
	halfHeight := float64(p.heightPx / 2)
	return (x-halfWidth)/p.pxPerMm + p.centre.X, -((y-halfHeight)/p.pxPerMm + p.centre.Y)
}
