package main

import (
	"fmt"
	"image/color"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
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
	th.Palette.ContrastBg = color.NRGBA{R: 75, G: 150, B: 150, A: 255}
	th.Palette.Fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	th.Palette.ContrastFg = color.NRGBA{R: 100, G: 255, B: 255, A: 255}

	var ops op.Ops

	editor := widget.Editor{
		SingleLine: true,
		Submit:     true,
	}

	for {
		e := w.NextEvent()
		switch e := e.(type) {
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			// fill with background colour
			paint.Fill(&ops, th.Palette.Bg)

			// ... UI goes here ...
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					return drawLabel(th, gtx)
				}),
				layout.Flexed(0.5, func(gtx C) D {
					return drawImage(gtx, img)
				}),
				layout.Flexed(0.5, func(gtx C) D {
					ed := material.Editor(th, &editor, "")
					ed.Font = gofont.Collection()[6].Font
					return ed.Layout(gtx)
				}),
			)

			for _, e := range editor.Events() {
				switch e.(type) {
				case widget.SubmitEvent:
					fmt.Printf(" > [%s]\n", editor.Text())
					editor.SetText("")
				}
			}

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
