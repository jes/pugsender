package main

import (
	"fmt"
	"image"
	"os"
	"strings"
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
	g     *Grbl
	gs    GrblStatus
	gsNew GrblStatus

	th              *material.Theme
	InitialTextSize unit.Sp
	w               *app.Window
	mode            Mode
	modeStack       []Mode
	autoConnect     bool
	jog             JogControl

	xDro                EditableNum
	yDro                EditableNum
	zDro                EditableNum
	aDro                EditableNum
	jogIncEdit          EditableNum
	jogFeedEdit         EditableNum
	jogRapidFeedEdit    EditableNum
	feedOverrideEdit    EditableNum
	rapidOverrideEdit   EditableNum
	spindleOverrideEdit EditableNum

	openBtn   *widget.Clickable
	startBtn  *widget.Clickable
	holdBtn   *widget.Clickable
	resetBtn  *widget.Clickable
	drainBtn  *widget.Clickable
	singleBtn *widget.Clickable
	unlockBtn *widget.Clickable
	m1Btn     *widget.Clickable

	tp *ToolpathView

	canUndo bool
	undoWco V4d

	numpop     *NumPop
	numpopType string

	split1 Split
	split2 Split

	gcode           *GCodeRunner
	gcodeRunnerChan chan RunnerCmd

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
		g:               NewGrbl(nil, "/dev/null"),
		mode:            ModeConnect,
		th:              th,
		autoConnect:     true,
		gcodeRunnerChan: make(chan RunnerCmd),
	}
	a.InitialTextSize = th.TextSize

	a.gcode = NewGCodeRunner(a)

	a.gsNew = DefaultGrblStatus()

	a.mdi = NewMDI(a)
	a.jog = NewJogControl(a)
	a.tp = NewToolpathView(a)
	a.split1.Ratio = -0.25
	a.split1.InvisibleBar = true
	a.split2.Ratio = 0
	a.split2.InvisibleBar = true

	a.xDro.app = a
	a.xDro.Label = "X"
	a.xDro.Callback = func(v float64) {
		w := a.gs.Wpos
		w.X = v
		a.SetWpos(w)
	}
	a.yDro.app = a
	a.yDro.Label = "Y"
	a.yDro.Callback = func(v float64) {
		w := a.gs.Wpos
		w.Y = v
		a.SetWpos(w)
	}
	a.zDro.app = a
	a.zDro.Label = "Z"
	a.zDro.Callback = func(v float64) {
		w := a.gs.Wpos
		w.Z = v
		a.SetWpos(w)
	}
	a.aDro.app = a
	a.aDro.Label = "A"
	a.aDro.Callback = func(v float64) {
		w := a.gs.Wpos
		w.A = v
		a.SetWpos(w)
	}
	a.jogIncEdit.app = a
	a.jogIncEdit.Label = " Inc."
	a.jogIncEdit.Callback = func(v float64) {
		a.jog.Increment = v
	}
	a.jogFeedEdit.app = a
	a.jogFeedEdit.Label = " Feed"
	a.jogFeedEdit.Callback = func(v float64) {
		a.jog.FeedRate = v
	}
	a.jogRapidFeedEdit.app = a
	a.jogRapidFeedEdit.Label = "Rapid"
	a.jogRapidFeedEdit.Callback = func(v float64) {
		a.jog.RapidFeedRate = v
	}
	a.feedOverrideEdit.app = a
	a.feedOverrideEdit.Label = "   Feed"
	a.feedOverrideEdit.Int = true
	a.feedOverrideEdit.Callback = func(v float64) {
		a.g.SetFeedOverride(int(v))
	}
	a.rapidOverrideEdit.app = a
	a.rapidOverrideEdit.Label = "  Rapid"
	a.rapidOverrideEdit.Int = true
	a.rapidOverrideEdit.Callback = func(v float64) {
		a.g.SetRapidOverride(int(v))
	}
	a.spindleOverrideEdit.app = a
	a.spindleOverrideEdit.Label = "Spindle"
	a.spindleOverrideEdit.Int = true
	a.spindleOverrideEdit.Callback = func(v float64) {
		a.g.SetSpindleOverride(int(v))
	}

	a.openBtn = new(widget.Clickable)
	a.startBtn = new(widget.Clickable)
	a.holdBtn = new(widget.Clickable)
	a.resetBtn = new(widget.Clickable)
	a.drainBtn = new(widget.Clickable)
	a.singleBtn = new(widget.Clickable)
	a.unlockBtn = new(widget.Clickable)
	a.m1Btn = new(widget.Clickable)

	var err error
	a.img, err = loadImage("pugs.png")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pugs.png: %v", err)
		os.Exit(1)
	}

	// initialise other text sizes
	a.SetTextSize(a.th.TextSize)

	a.w = app.NewWindow(
		app.Title("G-code sender"),
		app.Size(unit.Dp(1024), unit.Dp(768)),
	)

	return a
}

func (a *App) Run() {
	go a.jog.Run()
	go a.gcode.Run(a.gcodeRunnerChan)

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
						if keystate[gtxE.Name] == JogKeyRelease {
							// XXX: if a key is released and then pressed in the same frame, we take it to be held; is this right?
							keystate[gtxE.Name] = JogKeyHold
						} else {
							keystate[gtxE.Name] = JogKeyPress
						}

						if gtxE.Name == key.NameShift {
							a.jog.SetActiveFeedRate(a.jog.RapidFeedRate)
						}
					} else if gtxE.State == key.Release {
						keystate[gtxE.Name] = JogKeyRelease

						if gtxE.Name == key.NameShift {
							a.jog.SetActiveFeedRate(a.jog.FeedRate)
						}
					}
				case pointer.Event:
					if gtxE.Kind == pointer.Press {
						a.mdi.Defocus()
					} else if gtxE.Kind == pointer.Scroll {
						if gtxE.Modifiers.Contain(key.ModCtrl) {
							a.SetTextSize(a.th.TextSize * unit.Sp(1.0-float64(gtxE.Scroll.Y)/100.0))
						}
					}
				}
			}

			// update jog control
			if a.mode == ModeJog && a.CanJog() {
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
			a.WriteConf(a.gs)
			os.Exit(0)
		}
	}
}

func (a *App) Connect(g *Grbl, ch chan GrblStatus) {
	a.g = g
	a.gsNew = g.status
	go a.ReadConf()

	// write the current work coordinates to disk once per second
	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			<-ticker.C
			if !a.gs.Closed {
				a.WriteConf(a.gs)
			}
		}
	}()

	// receive status updates from grbl, store in a.gsNew, they'll be
	// moved into a.gs at the start of the next frame
	go func() {
		for {
			a.gsNew = <-ch
			if a.gsNew.Closed {
				a.ResetMode(ModeConnect)
			} else if a.mode == ModeConnect {
				a.ResetMode(ModeJog)
			}
			a.w.Invalidate()
			if a.gsNew.Closed {
				return
			}

			if a.gcode.stopping {
				// XXX: let the gcode runner discover a "Hold:0" status
				a.gcodeRunnerChan <- CmdNone
			}
		}
	}()
}

func (a *App) Layout(gtx C) D {
	// set the new GrblStatus at the start of Layout(), so that it doesn't
	// change mid-layout
	a.gs = a.gsNew

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

func (a *App) AlarmUnlock() {
	a.g.CommandIgnore("$X")
}

func (a *App) LayoutButtons(gtx C) D {
	for a.openBtn.Clicked(gtx) {
		a.OpenFile()
	}
	for a.startBtn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdStart
	}
	for a.holdBtn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdPause
	}
	for a.resetBtn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdStop
	}
	for a.drainBtn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdDrain
	}
	for a.singleBtn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdSingle
	}
	for a.unlockBtn.Clicked(gtx) {
		a.AlarmUnlock()
	}
	for a.m1Btn.Clicked(gtx) {
		a.gcodeRunnerChan <- CmdOptionalStop
	}

	m1Lbl := "+M1"
	if a.gcode.optionalStop {
		m1Lbl = "-M1"
	}

	return Toolbar{Inset: layout.UniformInset(5)}.Layout(gtx,
		material.Button(a.th, a.openBtn, "OPEN").Layout,
		material.Button(a.th, a.startBtn, "RUN").Layout,
		material.Button(a.th, a.holdBtn, "HOLD").Layout,
		material.Button(a.th, a.resetBtn, "STOP").Layout,
		material.Button(a.th, a.drainBtn, "DRAIN").Layout,
		material.Button(a.th, a.singleBtn, "SINGLE").Layout,
		material.Button(a.th, a.unlockBtn, "UNLOCK").Layout,
		material.Button(a.th, a.m1Btn, m1Lbl).Layout,
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
			// open gcode file
			a.OpenFile()
		} else if e.Name == "I" {
			// edit jog increment
			a.jogIncEdit.ShowEditor()
		} else if e.Name == "F" {
			// edit jog feed
			a.jogFeedEdit.ShowEditor()
		} else if e.Name == "P" {
			// edit fast jog feed
			a.jogRapidFeedEdit.ShowEditor()
		}
	}

	if a.mode == ModeJog || a.mode == ModeRun {
		if e.Name == "H" {
			// feed hold
			a.gcodeRunnerChan <- CmdPause
		} else if e.Name == "R" {
			// soft reset
			a.gcodeRunnerChan <- CmdStop
		} else if e.Name == "S" {
			// cycle start
			a.gcodeRunnerChan <- CmdStart
		} else if e.Name == "U" {
			// alarm unlock
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
				if a.canUndo {
					a.SetWpos(a.gs.Mpos.Sub(a.undoWco))
				}
			} else {
				a.zDro.ShowEditor()
			}
		} else if e.Name == "A" && a.gs.Has4thAxis {
			a.aDro.ShowEditor()
		}
	}

	if e.Name == key.NameEscape {
		// pop mode
		if a.mode == ModeMDI {
			a.mdi.Defocus()
		}
		if a.mode != ModeRun {
			a.PopMode()
		}
	} else if e.Name == "+" && e.Modifiers.Contain(key.ModCtrl) {
		// ctrl + = zoom in
		a.SetTextSize(a.th.TextSize * 1.1)
	} else if e.Name == "-" && e.Modifiers.Contain(key.ModCtrl) {
		// ctrl - = zoom out
		a.SetTextSize(a.th.TextSize / 1.1)
	} else if e.Name == "0" && e.Modifiers.Contain(key.ModCtrl) {
		// ctrl 0 = reset zoom
		a.SetTextSize(a.InitialTextSize)
	}
}

func (a *App) OpenFile() {
	go func() {
		w := app.NewWindow(app.Title("Open G-code file"))
		e := explorer.NewExplorer(w)
		f, err := e.ChooseFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "explorer.ChooseFile(): %v\n", err)
		} else {
			a.gcode.Load(f)
		}
	}()
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
	wco := a.gs.Wco
	if a.g.SetWpos(p) {
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

func (a *App) CanJog() bool {
	// we can jog if grbl is in "Idle" or "Jog" status
	return a.gs.Status == "Idle" || a.gs.Status == "Jog"
}

func (a *App) SetTextSize(sz unit.Sp) {
	a.th.TextSize = sz
	a.xDro.TextSize = a.th.TextSize * 2.16
	a.yDro.TextSize = a.th.TextSize * 2.16
	a.zDro.TextSize = a.th.TextSize * 2.16
	a.aDro.TextSize = a.th.TextSize * 2.16
	a.jogIncEdit.TextSize = a.th.TextSize * 1.6
	a.jogFeedEdit.TextSize = a.th.TextSize * 1.6
	a.jogRapidFeedEdit.TextSize = a.th.TextSize * 1.6
	a.feedOverrideEdit.TextSize = a.th.TextSize * 1.6
	a.rapidOverrideEdit.TextSize = a.th.TextSize * 1.6
	a.spindleOverrideEdit.TextSize = a.th.TextSize * 1.6
}
