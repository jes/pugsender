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
	showGridLines bool
	crossHair     V4d
	pxPerMm       float64
	centre        V4d // what coordinate is in the centre?
	widthPx       int
	heightPx      int
	axes          V4d
	Image         image.Image
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
func (p *Path) Render() {
	img := image.NewRGBA(image.Rect(0, 0, p.widthPx, p.heightPx))
	gc := draw2dimg.NewGraphicContext(img)

	centrex, centrey := p.MmToPx(p.axes.X, p.axes.Y)

	if p.showGridLines {
		// we want to draw a "solid" grid line every 10-100 pixels
		// at the current zoom level, at multiples of 10,
		// and a "fading in" grid line at 1/10 of that
		minSpacing := 100.0 // pixels between lines
		spacingMm := roundUpToNextPowerOfTen(minSpacing / p.pxPerMm)
		// convert back to pixels
		spacingPx := spacingMm * p.pxPerMm

		p.DrawGridLines(gc, spacingPx, grey(64))
		secondaryIntensity := interp(spacingPx, minSpacing, minSpacing*10)
		secondaryCol := grey(uint8(16 + float64(48)*secondaryIntensity))
		p.DrawGridLines(gc, spacingPx*0.1, secondaryCol)
	}

	if p.showAxes {
		p.DrawVLine(gc, math.Floor(centrex), rgb(64, 0, 0))
		p.DrawHLine(gc, math.Floor(centrey), rgb(0, 64, 0))
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
		x, y := p.MmToPx(p.crossHair.X, p.crossHair.Y)
		p.DrawCrossHair(gc, x, y, 12)
	}

	p.Image = img
}

func (p *Path) DrawGridLines(gc *draw2dimg.GraphicContext, step float64, col color.NRGBA) {
	centrex, centrey := p.MmToPx(p.axes.X, p.axes.Y)
	x0 := f64mod(centrex, step)
	y0 := f64mod(centrey, step)
	for x := x0; x <= float64(p.widthPx); x += step {
		p.DrawVLine(gc, math.Floor(x), col)
	}
	for y := y0; y <= float64(p.heightPx); y += step {
		p.DrawHLine(gc, math.Floor(y), col)
	}
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

func (p *Path) DrawVLine(gc *draw2dimg.GraphicContext, x float64, col color.NRGBA) {
	gc.SetStrokeColor(col)
	gc.MoveTo(x, 0)
	gc.LineTo(x, float64(p.heightPx))
	gc.Stroke()
}

func (p *Path) DrawHLine(gc *draw2dimg.GraphicContext, y float64, col color.NRGBA) {
	gc.SetStrokeColor(col)
	gc.MoveTo(0, y)
	gc.LineTo(float64(p.widthPx), y)
	gc.Stroke()
}

func roundUpToNextPowerOfTen(num float64) float64 {
	if num == 0 {
		return num
	}
	return math.Pow(10, math.Ceil(math.Log10(num)))
}

func interp(v, a, b float64) float64 {
	return (v - a) / (b - a)
}

func f64mod(a, b float64) float64 {
	return a - b*math.Floor(a/b)
}
