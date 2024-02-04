package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/256dpi/gcode"
)

func (a *App) LoadGCode(r io.Reader) {
	gcode := make([]string, 0)

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		gcode = append(gcode, line)
	}

	a.gcode = gcode
	a.nextLine = 0

	// TODO: this is plotted in the wrong place unless WCO=0
	a.tp.path.SetGCode(GCodeToPath(a.gcode))
}

func (a *App) CycleStart() {
	a.g.CommandRealtime('~')

	a.wantToRunGCode = true
	if a.runningGCode {
		return
	}
	a.runningGCode = true

	a.PushMode(ModeRun)

	go func() {
		for a.wantToRunGCode && a.nextLine < len(a.gcode) {
			line := a.gcode[a.nextLine]
			a.nextLine += 1

			fmt.Printf("> [%s]\n", line)
			// TODO: use the character-counting method instead of waiting for a response?
			a.g.CommandWait(line)

			// TODO: stop requesting G codes after every command (but
			// how else do we display up-to-date G codes?)
			a.g.RequestGCodes()
		}

		// reset after finished
		a.nextLine = 0
		a.runningGCode = false
		a.wantToRunGCode = false

		if a.mode == ModeRun {
			a.PopMode()
		}
	}()
}

func (a *App) SoftReset() {
	a.wantToRunGCode = false
	a.g.AbortCommands()
	a.g.CommandRealtime(0x18)
}

func (a *App) FeedHold() {
	a.g.CommandRealtime('!')
}

func (a *App) AlarmUnlock() {
	a.g.CommandIgnore("$X")
}

func GCodeToPath(lines []string) []V4d {
	pos := V4d{}

	path := make([]V4d, 0)

	for _, str := range lines {
		line, err := gcode.ParseLine(str)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing gcode line: [%s]: %s, ignoring\n", str, err)
			continue
		}
		G := -1
		// TODO: this is wrong, because it will update pos for non-movement lines
		for _, gc := range line.Codes {
			if gc.Letter == "G" {
				G = int(gc.Value)
			} else if gc.Letter == "X" {
				pos.X = gc.Value
			} else if gc.Letter == "Y" {
				pos.Y = gc.Value
			} else if gc.Letter == "Z" {
				pos.Z = gc.Value
			} else if gc.Letter == "A" {
				pos.A = gc.Value
			}
		}
		// TODO: G2,G3 need to be arcs, other movements need supporting
		if G == 0 || G == 1 || G == 2 || G == 3 {
			path = append(path, pos)
		}
	}

	return path
}
