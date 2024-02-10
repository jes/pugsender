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
	"gioui.org/widget"
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

	xDro             EditableNum
	yDro             EditableNum
	zDro             EditableNum
	aDro             EditableNum
	jogIncEdit       EditableNum
	jogFeedEdit      EditableNum
	jogRapidFeedEdit EditableNum

	startBtn *widget.Clickable
	holdBtn  *widget.Clickable
	resetBtn *widget.Clickable

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
	th.Palette.ContrastBg = grey(32)
	th.Palette.Fg = grey(255)
	th.Palette.ContrastFg = grey(255)

	a := &App{
		g:           NewGrbl(nil, "/dev/null"),
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

	a.xDro.app = a
	a.xDro.Label = "X"
	a.xDro.TextSize = th.TextSize * 2.16
	a.xDro.Callback = func(v float64) {
		w := a.g.Wpos
		w.X = v
		a.SetWpos(w)
	}
	a.yDro.app = a
	a.yDro.Label = "Y"
	a.yDro.TextSize = th.TextSize * 2.16
	a.yDro.Callback = func(v float64) {
		w := a.g.Wpos
		w.Y = v
		a.SetWpos(w)
	}
	a.zDro.app = a
	a.zDro.Label = "Z"
	a.zDro.TextSize = th.TextSize * 2.16
	a.zDro.Callback = func(v float64) {
		w := a.g.Wpos
		w.Z = v
		a.SetWpos(w)
	}
	a.aDro.app = a
	a.aDro.Label = "A"
	a.aDro.TextSize = th.TextSize * 2.16
	a.aDro.Callback = func(v float64) {
		w := a.g.Wpos
		w.A = v
		a.SetWpos(w)
	}
	a.jogIncEdit.app = a
	a.jogIncEdit.Label = " Inc."
	a.jogIncEdit.TextSize = th.TextSize * 1.6
	a.jogIncEdit.Callback = func(v float64) {
		a.jog.Increment = v
	}
	a.jogFeedEdit.app = a
	a.jogFeedEdit.Label = " Feed"
	a.jogFeedEdit.TextSize = th.TextSize * 1.6
	a.jogFeedEdit.Callback = func(v float64) {
		a.jog.FeedRate = v
	}
	a.jogRapidFeedEdit.app = a
	a.jogRapidFeedEdit.Label = "Rapid"
	a.jogRapidFeedEdit.TextSize = th.TextSize * 1.6
	a.jogRapidFeedEdit.Callback = func(v float64) {
		a.jog.RapidFeedRate = v
	}

	a.startBtn = new(widget.Clickable)
	a.holdBtn = new(widget.Clickable)
	a.resetBtn = new(widget.Clickable)

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

						if gtxE.Name == key.NameShift {
							a.jog.ActiveFeedRate = a.jog.RapidFeedRate
							// TODO: some way to "refresh jog feed rate" that does nothing if not jogging
							a.jog.JogTo(a.jog.Target.X, a.jog.Target.Y)
						}
					} else if gtxE.State == key.Release {
						keystate[gtxE.Name] = JogKeyRelease

						if gtxE.Name == key.NameShift {
							a.jog.ActiveFeedRate = a.jog.FeedRate
							// TODO: some way to "refresh jog feed rate" that does nothing if not jogging
							a.jog.JogTo(a.jog.Target.X, a.jog.Target.Y)
						}
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
				"(Ctrl)-+", "(Ctrl)--", "(Shift)-S", "(Shift)-R", "(Shift)-H", "(Shift)-X", "(Shift)-Y", "(Shift)-Z", "(Shift)-A", "(Shift)-G", "(Shift)-M", "(Shift)-J", "(Shift)-O", "(Shift)-I", "(Shift)-F", "(Shift)-U", "(Shift)-P", key.NameEscape, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown, key.NameShift,
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
						layout.Rigid(a.LayoutButtons),
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

func (a *App) LayoutButtons(gtx C) D {
	for a.startBtn.Clicked(gtx) {
		a.CycleStart()
	}
	for a.holdBtn.Clicked(gtx) {
		a.FeedHold()
	}
	for a.resetBtn.Clicked(gtx) {
		a.SoftReset()
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.UniformInset(5).Layout(gtx, material.Button(a.th, a.startBtn, "START").Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.UniformInset(5).Layout(gtx, material.Button(a.th, a.holdBtn, "HOLD").Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.UniformInset(5).Layout(gtx, material.Button(a.th, a.resetBtn, "RESET").Layout)
		}),
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
		} else if e.Name == "I" {
			a.jogIncEdit.ShowEditor()
		} else if e.Name == "F" {
			a.jogFeedEdit.ShowEditor()
		} else if e.Name == "P" {
			a.jogRapidFeedEdit.ShowEditor()
		}
	}

	if a.mode == ModeJog || a.mode == ModeRun {
		if e.Name == "H" {
			a.FeedHold()
		} else if e.Name == "R" {
			a.SoftReset()
		} else if e.Name == "S" {
			a.CycleStart()
		} else if e.Name == "U" {
			a.AlarmUnlock()
		}
	}

	if a.mode == ModeJog {
		// JOG MODE
		if e.Name == "X" {
			a.xDro.ShowEditor()
		} else if e.Name == "Y" {
			a.yDro.ShowEditor()
		} else if e.Name == "Z" {
			if e.Modifiers.Contain(key.ModCtrl) {
				// ctrl-z = undo WCO change
				// TODO: undo other operations?
				// TODO: more levels of undo?
				if a.canUndo {
					a.SetWpos(a.g.Mpos.Sub(a.undoWco))
				}
			} else {
				a.zDro.ShowEditor()
			}
		} else if e.Name == "A" {
			// TODO: only if there is a 4th axis
			a.aDro.ShowEditor()
		}
	}

	if e.Name == key.NameEscape {
		if a.mode == ModeMDI {
			a.mdi.Defocus()
		}
		if a.mode != ModeRun {
			a.PopMode()
		}
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
	a.numpop = NewNumPop(a.th, initVal, func(apply bool, val float64) {
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
	monos := make([]font.FontFace, 0)
	// look for monospace fonts
	for _, font := range fonts {
		if strings.Contains(strings.ToLower(string(font.Font.Typeface)), "mono") {
			monos = append(monos, font)
		}
	}
	if len(monos) > 0 {
		// return monospace fonts, if any
		return monos
	} else {
		// otherwise return all available fonts
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

func (a *App) ShowEditor() bool {
	if a.mode == ModeJog || a.mode == ModeRun || a.mode == ModeMDI {
		a.PushMode(ModeNum)
		return true
	} else {
		return false
	}
}

func (a *App) ShowingEditor() bool {
	return a.mode == ModeNum
}

func (a *App) EditorHidden() {
	if a.mode == ModeNum {
		a.PopMode()
	}
}
