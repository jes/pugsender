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
	// TODO: pause/resume, stop, etc.
	go func() {
		for a.nextLine < len(a.gcode) {
			fmt.Printf("> [%s]\n", a.gcode[a.nextLine])
			a.g.CommandWait(a.gcode[a.nextLine])
			a.nextLine += 1
		}
	}()
}
