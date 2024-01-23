package main

import (
	"fmt"
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
	app          *App
	keyHeld      map[string]bool
	keyHeldLock  sync.RWMutex
	FeedRate     float64
	Increment    float64
	TickerPeriod time.Duration
	HaveJogged   bool
}

func NewJogControl(app *App) JogControl {
	return JogControl{
		app:          app,
		keyHeld:      make(map[string]bool),
		FeedRate:     100,
		Increment:    1,
		TickerPeriod: 100 * time.Millisecond,
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
		j.keyHeldLock.RLock()
		j.SingleContinuous()
		j.keyHeldLock.RUnlock()
	}
}

func (j *JogControl) SingleContinuous() {
	jogDir := make(map[string]int)
	jogDir["X"] = 0
	jogDir["Y"] = 0
	jogDir["Z"] = 0
	jogDir["A"] = 0

	for k, held := range j.keyHeld {
		if held {
			valid, axis, dir := JogAction(k)
			if valid {
				// holding + and - will cancel each other out, which is what we want
				jogDir[axis] += dir
			}
		}
	}

	jogLine := "$J=G91"
	anyJogs := false
	jogDist := j.FeedRate * j.TickerPeriod.Minutes()

	for axis, dir := range jogDir {
		if dir != 0 {
			anyJogs = true
			jogLine += fmt.Sprintf("%s%.3f", axis, float64(dir)*jogDist)
		}
	}

	if anyJogs {
		jogLine += fmt.Sprintf("F%.3f", j.FeedRate)
		if c := j.app.g.Command(jogLine); c != nil {
			j.HaveJogged = true
			resp := <-c
			if resp != "ok" {
				fmt.Fprintf(os.Stderr, "error response to jog command [%s]: %s, ignoring\n", jogLine, resp)
			}
		} else {
			fmt.Fprintf(os.Stderr, "queue full while trying to jog, ignoring\n")
		}
	}

}

func (j *JogControl) Incremental(axis string, dir int) {
	ok := j.app.g.CommandIgnore(fmt.Sprintf("$J=G91%s%.3fF%.3f", axis, float64(dir)*j.Increment, j.FeedRate))
	if ok {
		j.HaveJogged = true
	}
}

func (j *JogControl) StartContinuous(axis string, dir int) {
	j.Cancel()
	j.SingleContinuous()
}

func (j *JogControl) JogTo(x, y float64) {
	j.Cancel()
	ok := j.app.g.CommandIgnore(fmt.Sprintf("$J=G90X%.3fY%.3fF%.3f", x, y, j.FeedRate))
	if ok {
		j.HaveJogged = true
	}
}

func (j *JogControl) Update(newKeyState map[string]JogKeyState) {
	j.keyHeldLock.Lock()
	defer j.keyHeldLock.Unlock()

	for k, state := range newKeyState {
		isJogAction, axis, dir := JogAction(k)
		if !isJogAction {
			continue
		}

		if state == JogKeyPress {
			j.keyHeld[k] = false
			j.Incremental(axis, dir)
		} else if state == JogKeyRelease {
			if j.KeyHeld(k) {
				j.keyHeld[k] = false
				j.Cancel()
				// TODO: resume jogs for held keys
				// TODO: if we were part way through an incomplete incremental jog, recreate the incremental jog
			}
		} else if state == JogKeyHold {
			if !j.KeyHeld(k) {
				j.keyHeld[k] = true
				j.StartContinuous(axis, dir)
			}
		}
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
