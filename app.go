package main

import (
	"fmt"
	"image"
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
	"gioui.org/widget/material"
)

type Mode int

const (
	ModeDisconnected Mode = iota
	ModeNormal
	ModeJog
	ModeMDI
	ModeMDISingle
)

func (m Mode) String() string {
	if m == ModeDisconnected {
		return "DISCONNECTED"
	} else if m == ModeNormal {
		return "NOR"
	} else if m == ModeJog {
		return "JOG"
	} else if m == ModeMDI {
		return "MDI"
	} else if m == ModeMDISingle {
		return "MDI"
	} else {
		return "???"
	}
}

type App struct {
	g           *Grbl
	th          *material.Theme
	w           *app.Window
	mode        Mode
	modeStack   []Mode
	autoConnect bool

	img image.Image
	mdi *MDI
}

func NewApp() *App {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette.Bg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	th.Palette.ContrastBg = color.NRGBA{R: 75, G: 150, B: 150, A: 255}
	th.Palette.Fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	th.Palette.ContrastFg = color.NRGBA{R: 100, G: 255, B: 255, A: 255}

	a := &App{
		g:           NewGrbl(nil, "<nil>"),
		mode:        ModeDisconnected,
		th:          th,
		autoConnect: true,
	}
	a.mdi = NewMDI(a)

	var err error
	a.img, err = loadImage("pugs.png")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pugs.png: %v", err)
		os.Exit(1)
	}

	a.w = app.NewWindow(
		app.Title("G-code sender"),
		app.Size(unit.Dp(800), unit.Dp(600)),
	)

	return a
}

func (a *App) Run() {
	var ops op.Ops

	for {
		e := a.w.NextEvent()
		switch e := e.(type) {
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			// fill with background colour
			paint.Fill(&ops, a.th.Palette.Bg)

			a.Layout(gtx)

			e.Frame(gtx.Ops)
		case system.DestroyEvent:
			os.Exit(0)
		}
	}
}

func (a *App) Connect(g *Grbl) {
	a.g = g
	go func() {
		for {
			<-a.g.StatusUpdate
			if a.g.Closed {
				a.ResetMode(ModeDisconnected)
			} else if a.mode == ModeDisconnected {
				a.ResetMode(ModeNormal)
			}
			a.w.Invalidate()
			if a.g.Closed {
				return
			}
		}
	}()
}

func (a *App) Layout(gtx C) D {

	if a.mdi.editor.Focused() && a.mode != ModeMDI && a.mode != ModeMDISingle {
		// TODO: should this push ModeMDISingle if mode == ModeJog?
		a.PushMode(ModeMDI)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// label at top
		layout.Flexed(1, func(gtx C) D {
			return drawDRO(a.th, gtx, a.g)
		}),
		// then an image
		layout.Rigid(func(gtx C) D {
			return drawImage(gtx, a.img)
		}),
		layout.Rigid(a.mdi.Layout),
		layout.Rigid(a.LayoutStatusBar),
	)
}

func (a *App) MDIInput(line string) {
	a.g.Write([]byte(line + "\n"))
	fmt.Printf(" > [%s]\n", line)
	if a.mode == ModeMDISingle {
		a.PopMode()
	}
}

func (a *App) PushMode(m Mode) {
	if m == a.mode {
		return
	}
	a.modeStack = append(a.modeStack, a.mode)
	a.mode = m
}

func (a *App) PopMode() {
	l := len(a.modeStack)
	if l > 0 {
		a.mode = a.modeStack[l-1]
		a.modeStack = a.modeStack[:l-1]
	} else {
		a.mode = ModeNormal
	}
}

func (a *App) ResetMode(m Mode) {
	a.mode = m
	a.modeStack = []Mode{}
}
