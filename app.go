package main

import (
	"fmt"
	"image"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Mode int

const (
	ModeConnect Mode = iota
	ModeJog
	ModeRun
	ModeMDI
)

func (m Mode) String() string {
	if m == ModeConnect {
		return "CON"
	} else if m == ModeJog {
		return "JOG"
	} else if m == ModeRun {
		return "RUN"
	} else if m == ModeMDI {
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
	jog         JogControl
	path        *Path

	img image.Image
	mdi *MDI
}

func NewApp() *App {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(chooseFonts(gofont.Collection())))
	th.Palette.Bg = grey(0)
	th.Palette.ContrastBg = rgb(75, 150, 150)
	th.Palette.Fg = grey(255)
	th.Palette.ContrastFg = rgb(100, 255, 255)

	a := &App{
		g:           NewGrbl(nil, "<nil>"),
		mode:        ModeConnect,
		th:          th,
		autoConnect: true,
	}
	a.mdi = NewMDI(a)
	a.jog = NewJogControl(a)
	a.path = NewPath()
	a.path.showCrossHair = true
	a.path.showAxes = true

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
	go a.jog.Run()

	var ops op.Ops

	for {
		e := a.w.NextEvent()
		switch e := e.(type) {
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			// handle keypresses
			keystate := make(map[string]JogKeyState)
			for _, gtxEvent := range gtx.Events(a) {
				switch gtxE := gtxEvent.(type) {
				case key.Event:
					if gtxE.State == key.Press {
						a.KeyPress(gtxE)
						if v, ok := keystate[gtxE.Name]; ok && v == JogKeyRelease {
							// XXX: if a key is released and then pressed in the same frame, we take it to be held; is this right?
							keystate[gtxE.Name] = JogKeyHold
						} else {
							keystate[gtxE.Name] = JogKeyPress
						}
					} else if gtxE.State == key.Release {
						keystate[gtxE.Name] = JogKeyRelease
					}
				case pointer.Event:
					if gtxE.Kind == pointer.Press {
						a.mdi.Defocus()
					} else if gtxE.Kind == pointer.Scroll {
						a.path.pxPerMm *= 1.0 - float64(gtxE.Scroll.Y)/100.0
					}
				}
			}

			// update jog control
			if a.mode == ModeJog {
				a.jog.Update(keystate)
			} else {
				a.jog.Cancel()
			}

			// fill with background colour
			paint.Fill(&ops, a.th.Palette.Bg)

			// ask for keyboard and mouse events in the whole window
			eventArea := clip.Rect(
				image.Rectangle{
					Min: image.Point{0, 0},
					Max: image.Point{gtx.Constraints.Max.X, gtx.Constraints.Max.Y},
				},
			).Push(gtx.Ops)

			keys := []string{
				"(Shift)-S", "(Shift)-R", "(Shift)-H", "(Shift)-G", "(Shift)-M", "(Shift)-J", key.NameEscape, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown,
			}
			key.InputOp{
				Keys: key.Set(strings.Join(keys, "|")),
				Tag:  a,
			}.Add(gtx.Ops)

			pointer.InputOp{
				Kinds:        pointer.Press | pointer.Scroll,
				Tag:          a,
				ScrollBounds: image.Rectangle{Min: image.Point{X: -500, Y: -500}, Max: image.Point{X: 500, Y: 500}},
			}.Add(gtx.Ops)

			// draw the application
			a.Layout(gtx)

			eventArea.Pop()

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
				a.ResetMode(ModeConnect)
			} else if a.mode == ModeConnect {
				a.ResetMode(ModeJog)
			}
			a.w.Invalidate()
			if a.g.Closed {
				return
			}
		}
	}()
}

func (a *App) Layout(gtx C) D {
	if a.mdi.editor.Focused() && a.mode != ModeMDI {
		a.PushMode(ModeMDI)
	}
	if !a.mdi.editor.Focused() && a.mode == ModeMDI {
		a.PopMode()
	}

	a.path.Update(a.g.Wpos)
	a.path.crossHair = a.g.WposExt()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					// not too wide, ...
					if gtx.Constraints.Max.X > 500 {
						gtx.Constraints.Max.X = 500
					}
					// ...but use 100% of available width
					gtx.Constraints.Min.X = gtx.Constraints.Max.X

					return Panel{Width: 1, Color: grey(128), Margin: 5, Padding: 5, BackgroundColor: grey(16), CornerRadius: 5}.Layout(gtx, func(gtx C) D {
						return a.LayoutDRO(gtx)
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					borderColour := rgb(128, 128, 128)
					return Panel{Margin: 5, Width: 1, CornerRadius: 5, Color: borderColour}.Layout(gtx, func(gtx C) D {
						// TODO: render in a different thread
						// TODO: panning, zooming
						// TODO: "jog to here"
						// TODO: show coordinates of hovered point
						if a.g.Vel.Length() > 0.001 {
							// invalidate the frame if the velocity is non-zero,
							// because we need to redraw the plotted path
							// XXX: we should instead invalidate only when the rendering thread has a new plot to show
							a.w.Invalidate()
						}
						img := a.path.Render(gtx.Constraints.Min.X, gtx.Constraints.Min.Y, V4d{})
						im := widget.Image{
							Src:   paint.NewImageOp(img),
							Scale: 1.0 / gtx.Metric.PxPerDp,
						}
						return im.Layout(gtx)
					})
				}),
			)
		}),
		layout.Rigid(a.mdi.Layout),
		layout.Rigid(a.LayoutStatusBar),
	)
}

func (a *App) MDIInput(line string) {
	a.g.Write([]byte(line + "\n"))
	fmt.Printf(" > [%s]\n", line)
	if a.mode == ModeMDI {
		a.mdi.Defocus()
		a.PopMode()
	}
}

func (a *App) PushMode(m Mode) {
	if m == a.mode {
		return
	}
	a.modeStack = append(a.modeStack, a.mode)
	a.mode = m
	a.w.Invalidate()
}

func (a *App) PopMode() {
	l := len(a.modeStack)
	if l > 0 {
		a.mode = a.modeStack[l-1]
		a.modeStack = a.modeStack[:l-1]
	} else {
		a.mode = ModeJog
	}
	a.w.Invalidate()
}

func (a *App) ResetMode(m Mode) {
	a.mode = m
	a.modeStack = []Mode{}
	if a.mdi.editor.Focused() {
		a.mdi.Defocus()
	}
	a.w.Invalidate()
}

func (a *App) KeyPress(e key.Event) {
	if a.mode == ModeJog {
		// JOG MODE
		if e.Name == "G" || e.Name == "M" {
			// enter MDI
			if a.mdi.editor.Text() == "" {
				a.mdi.editor.SetText(e.Name)
				a.mdi.editor.SetCaret(1, 1)
			}
			a.mdi.editor.Focus()
			a.PushMode(ModeMDI)
		} else if e.Name == "H" {
			// feed hold
			a.g.Write([]byte{'!'})
		} else if e.Name == "R" {
			// soft-reset
			a.g.Write([]byte{0x18})
		} else if e.Name == "S" {
			// cycle-start
			a.g.Write([]byte{'~'})
		}
	}

	if e.Name == key.NameEscape {
		if a.mode == ModeMDI {
			a.mdi.Defocus()
		}
		a.PopMode()
	}
}

func (a *App) Label(text string) Label {
	return Label{
		text: text,
		th:   a.th,
	}
}

func chooseFonts(fonts []font.FontFace) []font.FontFace {
	chosen := make([]font.FontFace, 0)
	for _, font := range fonts {
		if strings.Contains(strings.ToLower(string(font.Font.Typeface)), "mono") {
			chosen = append(chosen, font)
		}
	}
	if len(chosen) > 0 {
		return chosen
	} else {
		return fonts
	}
}
