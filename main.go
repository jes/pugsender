package main

import (
	"fmt"
	"image/color"
	"os"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func run() {
	img, err := loadImage("pugs.png")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pugs.png: %v", err)
		os.Exit(1)
	}

	w := app.NewWindow(
		app.Title("G-code sender"),
		app.Size(unit.Dp(800), unit.Dp(600)),
	)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette.Bg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	th.Palette.ContrastBg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	th.Palette.Fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	th.Palette.ContrastFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	var ops op.Ops

	for {
		e := w.NextEvent()
		switch e := e.(type) {
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			// fill with background colour
			paint.Fill(&ops, th.Palette.Bg)

			// ... UI goes here ...
			layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return drawLabel(th, gtx)
				}),
				layout.Flexed(1, func(gtx C) D {
					return drawImage(gtx, img)
				}),
			)

			e.Frame(gtx.Ops)
		case system.DestroyEvent:
			os.Exit(0)
		}
	}
}

func main() {
	go run()
	app.Main()
}
