package main

import (
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"time"

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
	"gioui.org/widget/material"
	"gioui.org/x/explorer"
)

type Mode int

const (
	ModeConnect Mode = iota
	ModeJog
	ModeRun
	ModeMDI
	ModeNum
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
	} else if m == ModeNum {
		return "NUM"
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

	tp *ToolpathView

	canUndo bool
	undoWco V4d

	numpop     *NumPop
	numpopType string

	confLock     sync.RWMutex
	canWriteConf bool

	split1 Split
	split2 Split

	gcode          []string
	nextLine       int
	runningGCode   bool
	wantToRunGCode bool

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
	a.tp = NewToolpathView(a)
	a.split1.Ratio = -0.5
	a.split1.InvisibleBar = true
	a.split2.Ratio = 0
	a.split2.InvisibleBar = true

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
						if gtxE.Modifiers.Contain(key.ModCtrl) {
							a.th.TextSize *= unit.Sp(1.0 - float64(gtxE.Scroll.Y)/100.0)
						}
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
				"(Ctrl)-+", "(Ctrl)--", "(Shift)-S", "(Shift)-R", "(Shift)-H", "(Shift)-X", "(Shift)-Y", "(Shift)-Z", "(Shift)-A", "(Shift)-G", "(Shift)-M", "(Shift)-J", "(Shift)-O", key.NameEscape, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown,
			}
			key.InputOp{
				Keys: key.Set(strings.Join(keys, "|")),
				Tag:  a,
			}.Add(gtx.Ops)

			pointer.InputOp{
				Kinds:        pointer.Press | pointer.Scroll,
				Tag:          a,
				ScrollBounds: image.Rectangle{Min: image.Point{X: -50, Y: -50}, Max: image.Point{X: 50, Y: 50}},
			}.Add(gtx.Ops)

			// draw the application
			a.Layout(gtx)

			eventArea.Pop()

			e.Frame(gtx.Ops)
		case system.DestroyEvent:
			// save work coordinates before exiting
			a.WriteConf()
			os.Exit(0)
		}
	}
}

func (a *App) Connect(g *Grbl) {
	a.g = g
	a.ReadConf()

	// write the current work coordinates to disk once per second
	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			<-ticker.C
			if !a.g.Closed {
				a.WriteConf()
			}
		}
	}()

	// receive status updates from grbl
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
			return a.split1.Layout(gtx, func(gtx C) D {
				return Panel{Width: 1, Color: grey(128), Margin: layout.UniformInset(5), Padding: layout.UniformInset(5), BackgroundColor: grey(16), CornerRadius: 5}.Layout(gtx, func(gtx C) D {
					return a.LayoutDRO(gtx)
				})

			}, func(gtx C) D {
				return a.split2.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return a.LayoutGCode(gtx)
						}),
						layout.Rigid(a.LayoutMDI),
					)
				},
					a.tp.Layout)
			})
		}),
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

func (a *App) MDIInput(line string) {
	a.g.CommandIgnore(line)
	fmt.Printf(" > [%s]\n", line)
	if a.mode == ModeMDI && a.mdi.defocusOnSubmit {
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
	if a.mode == ModeNum {
		// lose the reference to the NumPop when leaving ModeNum
		a.numpop = nil
	}
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
			a.mdi.defocusOnSubmit = true
			a.PushMode(ModeMDI)
		}
	}

	if a.mode == ModeJog {
		// JOG MODE
		if e.Name == "H" {
			a.FeedHold()
		} else if e.Name == "R" {
			a.SoftReset()
		} else if e.Name == "S" {
			a.CycleStart()
		} else if e.Name == "X" {
			a.ShowDROEditor("X", a.g.Wpos.X)
		} else if e.Name == "Y" {
			a.ShowDROEditor("Y", a.g.Wpos.Y)
		} else if e.Name == "Z" {
			if e.Modifiers.Contain(key.ModCtrl) {
				// ctrl-z = undo WCO change
				// TODO: undo other operations?
				// TODO: more levels of undo?
				if a.canUndo {
					a.SetWpos(a.g.Mpos.Sub(a.undoWco))
				}
			} else {
				a.ShowDROEditor("Z", a.g.Wpos.Z)
			}
		} else if e.Name == "A" {
			// TODO: only if there is a 4th axis
			a.ShowDROEditor("A", a.g.Wpos.A)
		} else if e.Name == "O" {
			// TODO: this is not exactly what I want, because:
			// - it lets you open multiple file browsers simultaneously
			// - it doesn't have the nice keyboard-driven tab-completing ui I want
			go func() {
				w := app.NewWindow(app.Title("Open G-code file"))
				e := explorer.NewExplorer(w)
				f, err := e.ChooseFile() // TODO: filtering by ".gcode" file extension doesn't seem to work properly? hides some files even if they have the correct extension
				if err != nil {
					fmt.Fprintf(os.Stderr, "explorer.ChooseFile(): %v\n", err)
				} else {
					a.LoadGCode(f)
				}
			}()
		}
	}

	if e.Name == key.NameEscape {
		if a.mode == ModeMDI {
			a.mdi.Defocus()
		}
		a.PopMode()
	} else if e.Name == "+" && e.Modifiers.Contain(key.ModCtrl) {
		a.th.TextSize *= 1.1
	} else if e.Name == "-" && e.Modifiers.Contain(key.ModCtrl) {
		a.th.TextSize /= 1.1
	} else if e.Name == "0" && e.Modifiers.Contain(key.ModCtrl) {
		// XXX: is this always right?
		a.th.TextSize = 16.0
	}
}

func (a *App) ShowNumPop(numpopType string, initVal float64, cb func(float64)) {
	a.numpop = NewNumPop(a, initVal, func(apply bool, val float64) {
		if apply {
			cb(val)
		}
		a.PopMode()
	})
	a.numpopType = numpopType
	a.PushMode(ModeNum)
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

// use this only for WCO changes initiated by the user (i.e. that they might want to undo); otherwise use a.g.SetWpos() directly
func (a *App) SetWpos(p V4d) {
	wco := a.g.Wco
	if a.g.SetWpos(p) {
		// TODO: popup a message saying they can use Ctrl-Z to undo
		a.undoWco = wco
		a.canUndo = true
	}
}
