package main

import (
	"image"
	"os"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

func drawImage(gtx layout.Context, img image.Image, zoom float64) D {
	im := widget.Image{
		Src:   paint.NewImageOp(img),
		Scale: float32(0.07 * zoom),
	}
	return im.Layout(gtx)
}

func loadImage(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	return im, err
}
