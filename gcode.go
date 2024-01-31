package main

import (
	"bufio"
	"fmt"
	"io"
)

func (a *App) LoadGcode(r io.Reader) {
	gcode := make([]string, 0)

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		gcode = append(gcode, line)
	}

	a.gcode = gcode
	a.nextLine = 0
}

func (a *App) CycleStart() {
	a.g.CommandRealtime('~')

	if a.runningGcode {
		return
	}
	a.wantToRunGcode = true
	a.runningGcode = true

	go func() {
		for a.wantToRunGcode && a.nextLine < len(a.gcode) {
			line := a.gcode[a.nextLine]
			a.nextLine += 1

			fmt.Printf("> [%s]\n", line)
			a.g.CommandWait(line)

			// TODO: stop requesting G codes after every command (but
			// how else do we display up-to-date G codes?)
			a.g.RequestGCodes()
		}

		// reset after finished
		a.nextLine = 0
		a.runningGcode = false
	}()
}

func (a *App) SoftReset() {
	a.wantToRunGcode = false
	a.g.CommandRealtime(0x18)
}

func (a *App) FeedHold() {
	a.g.CommandRealtime('!')
}
