package main

import (
	"fmt"
	"math"
	"os"
	"sync"
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
	keyHeld        map[string]bool
	keyHeldLock    sync.RWMutex
	Increment      float64
	FeedRate       float64
	RapidFeedRate  float64
	ActiveFeedRate float64 // will be either FeedRate or RapidFeedRate depending on whether Shift is pressed
	TickerPeriod   time.Duration
	LastJog        time.Time
	HaveJogged     bool
	Target         V4d
	Tick           chan struct{}
}

func NewJogControl(app *App) JogControl {
	return JogControl{
		app:            app,
		keyHeld:        make(map[string]bool),
		FeedRate:       100,
		RapidFeedRate:  1000,
		ActiveFeedRate: 100,
		Increment:      1,
		TickerPeriod:   100 * time.Millisecond,
		Tick:           make(chan struct{}),
	}
}

func (j *JogControl) Cancel() {
	if !j.HaveJogged {
		return
	}
	// 0x85 = Jog Cancel
	j.app.g.CommandRealtime(0x85)
	j.HaveJogged = false
}

func (j *JogControl) Run() {
	ticker := time.NewTicker(j.TickerPeriod)
	for {
		<-ticker.C

		// if no jogs have been sent in the last "gracePeriod",
		// reset the target for any axis that is not moving
		gracePeriod := 200 * time.Millisecond
		if time.Now().Sub(j.LastJog) > gracePeriod {
			eps := 0.001
			if math.Abs(j.app.g.Vel.X) < eps {
				j.Target.X = j.app.g.Wpos.X
			}
			if math.Abs(j.app.g.Vel.Y) < eps {
				j.Target.Y = j.app.g.Wpos.Y
			}
			if math.Abs(j.app.g.Vel.Z) < eps {
				j.Target.Z = j.app.g.Wpos.Z
			}
			if math.Abs(j.app.g.Vel.A) < eps {
				j.Target.A = j.app.g.Wpos.A
			}
		}

		j.SingleContinuous(false)
	}
}

func (j *JogControl) SendJogCommand(line string) bool {
	j.LastJog = time.Now()
	if j.app.g.PlannerFree < 2 {
		return false
	}
	fmt.Println(line)
	ok := j.app.g.CommandIgnore("$J=" + line)
	if ok {
		j.LastJog = time.Now()
		j.HaveJogged = true
		return true
	} else {
		fmt.Fprintf(os.Stderr, "BUG?? error [%s] while trying to jog, ignoring\n", line)
		return false
	}
}

func (j *JogControl) SingleContinuous(force bool) {
	j.keyHeldLock.RLock()
	defer j.keyHeldLock.RUnlock()

	jogDist := j.ActiveFeedRate * j.TickerPeriod.Minutes()

	anyJogs := false
	for k, held := range j.keyHeld {
		if held {
			valid, axis, dir := JogAction(k)
			if valid {
				j.AddIncrement(axis, dir, jogDist)
				anyJogs = true
			}
		}
	}

	if anyJogs || force {
		// TODO: support 4th axis jogging?
		j.SendJogCommand(fmt.Sprintf("X%.3fY%.3fZ%.3fF%.3f", j.Target.X, j.Target.Y, j.Target.Z, j.ActiveFeedRate))
	}
}

func (j *JogControl) AddIncrement(axis string, dir int, dist float64) float64 {
	inc := float64(dir) * dist
	if axis == "X" {
		j.Target.X += inc
		return j.Target.X
	} else if axis == "Y" {
		j.Target.Y += inc
		return j.Target.Y
	} else if axis == "Z" {
		j.Target.Z += inc
		return j.Target.Z
	} else if axis == "A" {
		j.Target.A += inc
		return j.Target.A
	} else {
		return 0.0
	}
}

func (j *JogControl) JogTo(x, y float64) {
	j.Cancel()
	j.SendJogCommand(fmt.Sprintf("X%.3fY%.3fF%.3f", x, y, j.ActiveFeedRate))
}

func (j *JogControl) Update(newKeyState map[string]JogKeyState) {
	j.keyHeldLock.Lock()

	needCancel := false
	needMove := false
	forceMove := false
	for k, state := range newKeyState {
		isJogAction, axis, dir := JogAction(k)
		if isJogAction {
			if state == JogKeyPress {
				j.AddIncrement(axis, dir, j.Increment)
				needMove = true
				forceMove = true
			} else if state == JogKeyRelease {
				if j.KeyHeld(k) {
					needCancel = true
					// TODO: if there are still other keys held, this doesn't cancel very effectively, what to do?
					// 1. cancel all continuous jogs and make them re-press to start the others?
					// 2. set the cancelled axis's jog target to the current Wpos? may cause it to reverse after it stops
					// 3. set the cancelled axis's jog target to the current Wpos, but continue to update it to the new Wpos every update until it stops?
				}
			} else if state == JogKeyHold {
				if !j.KeyHeld(k) {
					needMove = true
				}
			}
		}

		if state == JogKeyPress {
			j.keyHeld[k] = false
		} else if state == JogKeyRelease {
			j.keyHeld[k] = false
		} else if state == JogKeyHold {
			j.keyHeld[k] = true
		} else {
			fmt.Fprintf(os.Stderr, "BUG: unexpected key state: %d\n", state)
		}
	}

	j.keyHeldLock.Unlock()

	if needMove {
		j.Cancel()
		j.SingleContinuous(forceMove)
	} else if needCancel {
		j.Cancel()
	}
}

func (j *JogControl) KeyHeld(k string) bool {
	if v, ok := j.keyHeld[k]; ok {
		return v
	} else {
		return false
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
