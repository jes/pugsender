package main

import (
	"fmt"
	"os"
	"time"

	"gioui.org/io/key"
)

type JogKeyState int

const (
	JogKeyNone = iota
	JogKeyRelease
	JogKeyPress
	JogKeyHold
)

const (
	JogXNeg = key.NameLeftArrow
	JogXPos = key.NameRightArrow
	JogYNeg = key.NameDownArrow
	JogYPos = key.NameUpArrow
	JogZNeg = key.NamePageDown
	JogZPos = key.NamePageUp
	JogANeg = "...no such key..."
	JogAPos = "...no such key..."
)

type JogControl struct {
	app            *App
	Increment      float64
	FeedRate       float64
	RapidFeedRate  float64
	ActiveFeedRate float64 // will be either FeedRate or RapidFeedRate depending on whether Shift is pressed
	TickerPeriod   time.Duration
	HaveJogged     bool
	Target         V4d
	Axes           JogAxis4d
}

func NewJogControl(app *App) JogControl {
	return JogControl{
		app:            app,
		FeedRate:       100,
		RapidFeedRate:  1000,
		ActiveFeedRate: 100,
		Increment:      1,
		TickerPeriod:   100 * time.Millisecond,
	}
}

func (j *JogControl) Cancel() {
	if !j.HaveJogged {
		return
	}
	// 0x85 = Jog Cancel
	j.app.g.CommandRealtime(0x85)
	j.HaveJogged = false
	j.Axes.WasCancelled()
}

func (j *JogControl) Run() {
	ticker := time.NewTicker(j.TickerPeriod)
	for {
		<-ticker.C

		j.Axes.Update(j.app.g.Wpos, j.app.g.Vel)
		j.Axes.StepContinuous(j.ActiveFeedRate * j.TickerPeriod.Minutes())
		j.SendJog()
	}
}

func (j *JogControl) SendJog() {
	cmd := j.Axes.JogCommand()
	if len(cmd) == 0 {
		return
	}
	ok := j.SendJogCommand(cmd + fmt.Sprintf("F%.3f", j.ActiveFeedRate))
	if ok {
		j.Axes.SentCommand()
	}
}

func (j *JogControl) SendJogCommand(line string) bool {
	if len(line) == 0 {
		return true
	}
	if j.app.g.PlannerFree < 2 {
		return false
	}
	fmt.Println(line)
	ok := j.app.g.CommandIgnore("$J=" + line)
	if ok {
		j.HaveJogged = true
		return true
	} else {
		fmt.Fprintf(os.Stderr, "BUG?? error [%s] while trying to jog, ignoring\n", line)
		return false
	}
}

func (j *JogControl) JogTo(x, y float64) {
	// TODO: should this update the targets for the axes?
	j.Cancel()
	j.SendJogCommand(fmt.Sprintf("X%.3fY%.3fF%.3f", x, y, j.ActiveFeedRate))
}

func (j *JogControl) Update(newKeyState map[string]JogKeyState) {
	needCancel := false
	for k, state := range newKeyState {
		ok, axisName, dir := JogAction(k)
		if !ok {
			continue
		}

		axis := j.Axes.Select(axisName)
		if state == JogKeyPress {
			if dir == 1 {
				axis.AddIncremental(j.Increment)
				axis.UpKeyHeld = false
			} else {
				axis.AddIncremental(-j.Increment)
				axis.DownKeyHeld = false
			}
		} else if state == JogKeyRelease {
			if dir == 1 {
				if axis.UpKeyHeld {
					axis.UpKeyHeld = false
					needCancel = true
				}
			} else {
				if axis.DownKeyHeld {
					axis.DownKeyHeld = false
					needCancel = true
				}
			}
		} else if state == JogKeyHold {
			if dir == 1 {
				if !axis.UpKeyHeld {
					axis.UpKeyHeld = true
					axis.NeedsCommand = true
				}
			} else {
				if !axis.DownKeyHeld {
					axis.DownKeyHeld = true
					axis.NeedsCommand = true
				}
			}
		}
	}

	if needCancel || len(j.Axes.JogCommand()) > 0 {
		j.Cancel()
		j.SendJog()
	}
}

func JogAction(key string) (bool, string, int) {
	if key == JogXNeg {
		return true, "X", -1
	} else if key == JogXPos {
		return true, "X", +1
	} else if key == JogYNeg {
		return true, "Y", -1
	} else if key == JogYPos {
		return true, "Y", +1
	} else if key == JogZNeg {
		return true, "Z", -1
	} else if key == JogZPos {
		return true, "Z", +1
	} else if key == JogANeg {
		return true, "A", -1
	} else if key == JogAPos {
		return true, "A", +1
	} else {
		return false, "", 0
	}
}
