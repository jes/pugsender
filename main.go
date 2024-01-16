package main

import (
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

func run() {
	w := app.NewWindow(
		app.Title("G-code sender"),
		app.Size(unit.Dp(800), unit.Dp(600)),
	)

	th := material.NewTheme()
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
			label := material.H1(th, "Hello, world!")
			label.Layout(gtx)

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
