package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/llgcode/draw2d/draw2dimg"
)

const (
	MinPxPerMm = 0.1
	MaxPxPerMm = 10000.0
)

type PathOpts struct {
	showAxes      bool
	showCrossHair bool
	showGridLines bool
	crossHair     V4d
	pxPerMm       float64
	centre        V4d // what coordinate is in the centre?
	widthPx       int
	heightPx      int
	axes          V4d
}

type Path struct {
	PathOpts
	last PathOpts

	positions      []V4d
	drawnPositions int

	ForceRedraw bool

	Image           image.Image
	backgroundLayer *image.RGBA
	toolpathLayer   *image.RGBA
	foregroundLayer *image.RGBA
}

func NewPath() *Path {
	return &Path{
		PathOpts:    PathOpts{pxPerMm: 10},
		ForceRedraw: true,
	}
}

func (p *Path) Update(pos V4d) {
	// TODO: if this point lies on a straight line through
	// the last 2 points, simplify the path by replacing the
	// last point instead of appending
	eps := 0.001
	l := len(p.positions)
	if l > 0 && p.positions[l-1].Sub(pos).Length() < eps {
		// don't add a duplicate point
		return
	}
	p.positions = append(p.positions, pos)
}

// return true if redrawn, false if no changes to draw
func (p *Path) Render() bool {
	if p.pxPerMm > MaxPxPerMm {
		p.pxPerMm = MaxPxPerMm
	}
	if p.pxPerMm < MinPxPerMm {
		p.pxPerMm = MinPxPerMm
	}
	eps := 0.000001
	if p.widthPx != p.last.widthPx || p.heightPx != p.last.heightPx || math.Abs(p.pxPerMm-p.last.pxPerMm) > eps || p.centre.Sub(p.last.centre).Length() > eps {
		p.ForceRedraw = true
	}

	opts := p.PathOpts

	changed := false
	if p.RenderBackground() {
		changed = true
	}
	if p.RenderToolpath() {
		changed = true
	}
	if p.RenderForeground() {
		changed = true
	}

	p.ForceRedraw = false
	p.last = opts

	if !changed {
		return false
	}

	bounds := image.Rect(0, 0, p.widthPx, p.heightPx)
	composite := image.NewRGBA(bounds)
	draw.Draw(composite, bounds, p.backgroundLayer, image.Point{}, draw.Src)
	draw.Draw(composite, bounds, p.toolpathLayer, image.Point{}, draw.Over)
	draw.Draw(composite, bounds, p.foregroundLayer, image.Point{}, draw.Over)

	p.Image = composite

	return true
}

func (p *Path) RenderBackground() bool {
	eps := 0.000001
	if !p.ForceRedraw &&
		p.showAxes == p.last.showAxes &&
		p.showGridLines == p.last.showGridLines &&
		p.axes.Sub(p.last.axes).Length() < eps {
		// no need to re-render
		return false
	}

	p.backgroundLayer = image.NewRGBA(image.Rect(0, 0, p.widthPx, p.heightPx))
	gc := draw2dimg.NewGraphicContext(p.backgroundLayer)

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

	return true
}

func (p *Path) RenderToolpath() bool {
	l := len(p.positions)
	if !p.ForceRedraw &&
		p.drawnPositions == l {
		// no need to re-render
		return false
	}

	startIdx := p.drawnPositions - 1
	if p.ForceRedraw {
		p.toolpathLayer = image.NewRGBA(image.Rect(0, 0, p.widthPx, p.heightPx))
		startIdx = 0
	}
	gc := draw2dimg.NewGraphicContext(p.toolpathLayer)

	if l > 0 {
		gc.SetStrokeColor(color.White)
		gc.MoveTo(p.MmToPx(p.positions[startIdx].X, p.positions[startIdx].Y))
		for _, pos := range p.positions[startIdx+1:] {
			gc.LineTo(p.MmToPx(pos.X, pos.Y))
		}
		gc.Stroke()

		p.drawnPositions = l
	}

	return true
}

func (p *Path) RenderForeground() bool {
	eps := 0.000001
	if !p.ForceRedraw &&
		p.crossHair.Sub(p.last.crossHair).Length() < eps {
		// no need to re-render
		return false
	}

	p.foregroundLayer = image.NewRGBA(image.Rect(0, 0, p.widthPx, p.heightPx))
	gc := draw2dimg.NewGraphicContext(p.foregroundLayer)

	if p.showCrossHair {
		gc.SetStrokeColor(grey(128))
		x, y := p.MmToPx(p.crossHair.X, p.crossHair.Y)
		p.DrawCrossHair(gc, x, y, 12)
	}

	return true
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
