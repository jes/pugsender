package main

import (
	"fmt"
	"image"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/f32"
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

	dragStart       f32.Point
	dragStartCentre V4d
	dragging        bool

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
				Kinds: pointer.Press | pointer.Scroll,
				Tag:   a,
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
				layout.Flexed(1, a.LayoutToolpath),
			)
		}),
		layout.Rigid(a.LayoutMDI),
		layout.Rigid(a.LayoutStatusBar),
	)
}

func (a *App) LayoutMDI(gtx C) D {
	if a.mdi.editor.Focused() && a.mode != ModeMDI {
		a.PushMode(ModeMDI)
	}
	if !a.mdi.editor.Focused() && a.mode == ModeMDI {
		a.PopMode()
	}

	return a.mdi.Layout(gtx)
}

func (a *App) LayoutToolpath(gtx C) D {
	a.path.Update(a.g.Wpos)
	a.path.crossHair = a.g.WposExt()

	for _, gtxEvent := range gtx.Events(a.path) {
		switch gtxE := gtxEvent.(type) {
		case pointer.Event:
			if gtxE.Kind == pointer.Press {
				if gtxE.Modifiers.Contain(key.ModCtrl) {
					a.jog.JogTo(a.path.PxToMm(float64(gtxE.Position.X), float64(gtxE.Position.Y)))
				}
			} else if gtxE.Kind == pointer.Scroll {
				a.path.pxPerMm *= 1.0 - float64(gtxE.Scroll.Y)/100.0
			} else if gtxE.Kind == pointer.Drag {
				if !a.dragging {
					a.dragging = true
					a.dragStart = gtxE.Position
					a.dragStartCentre = a.path.centre
				}
				origCentre := f32.Point{X: float32(a.dragStartCentre.X), Y: float32(a.dragStartCentre.Y)}
				newCentre := origCentre.Add((a.dragStart.Sub(gtxE.Position)).Div(float32(a.path.pxPerMm)))
				a.path.centre = V4d{X: float64(newCentre.X), Y: float64(newCentre.Y)}
			} else if gtxE.Kind == pointer.Release {
				a.dragging = false
			}

		}
	}

	borderColour := rgb(128, 128, 128)
	dims := Panel{Margin: 5, Width: 1, CornerRadius: 5, Color: borderColour}.Layout(gtx, func(gtx C) D {
		// TODO: render in a different thread
		// TODO: "jog to here"
		// TODO: show coordinates of hovered point
		if a.g.Vel.Length() > 0.001 {
			// invalidate the frame if the velocity is non-zero,
			// because we need to redraw the plotted path
			// XXX: we should instead invalidate only when the rendering thread has a new plot to show
			a.w.Invalidate()
		}
		a.path.widthPx = gtx.Constraints.Min.X
		a.path.heightPx = gtx.Constraints.Min.Y
		img := a.path.Render()
		im := widget.Image{
			Src:   paint.NewImageOp(img),
			Scale: 1.0 / gtx.Metric.PxPerDp,
		}
		return im.Layout(gtx)
	})

	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	pointer.InputOp{
		Kinds:        pointer.Press | pointer.Scroll | pointer.Drag | pointer.Release,
		Tag:          a.path,
		ScrollBounds: image.Rectangle{Min: image.Point{X: -50, Y: -50}, Max: image.Point{X: 50, Y: 50}},
	}.Add(gtx.Ops)

	return dims
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
	if a.mode == ModeJog || a.mode == ModeConnect {
		if e.Name == "G" || e.Name == "M" {
			// enter MDI
			if a.mdi.editor.Text() == "" {
				a.mdi.editor.SetText(e.Name)
				a.mdi.editor.SetCaret(1, 1)
			}
			a.mdi.editor.Focus()
			a.PushMode(ModeMDI)
		}
	} else if a.mode == ModeJog {
		// JOG MODE
		if e.Name == "H" {
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
