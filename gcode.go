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

func (a *App) RunGcode() {
	if a.runningGcode {
		return
	}
	a.runningGcode = true

	// TODO: pause/resume, stop, etc.
	go func() {
		for a.runningGcode && a.nextLine < len(a.gcode) {
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
