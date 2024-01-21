package main

import (
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d/draw2dimg"
)

type Path struct {
	positions     []V4d
	showCrossHair bool
	crossHair     V4d
}

func NewPath() *Path {
	return &Path{}
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
func (p *Path) Render(w int, h int, centre V4d, pxPerMm float64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	l := len(p.positions)
	if l == 0 {
		return img
	}

	gc := draw2dimg.NewGraphicContext(img)
	gc.SetStrokeColor(color.White)
	gc.MoveTo(pxPerMm*(p.positions[0].X-centre.X)+float64(w/2), pxPerMm*(-p.positions[0].Y-centre.Y)+float64(h/2))
	for _, pos := range p.positions {
		gc.LineTo(pxPerMm*(pos.X-centre.X)+float64(w/2), pxPerMm*(-pos.Y-centre.Y)+float64(h/2))
	}
	gc.Stroke()

	if p.showCrossHair {
		x := p.crossHair.X - centre.X
		y := -p.crossHair.Y - centre.Y
		gc.SetStrokeColor(grey(128))
		p.DrawCrossHair(gc, pxPerMm*x+float64(w/2), pxPerMm*y+float64(h/2), 12)
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
