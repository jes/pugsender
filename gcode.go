package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/256dpi/gcode"
)

const (
	CmdNone = iota
	CmdStart
	CmdStop
	CmdPause
	CmdDrain
	CmdSingle
)

type RunnerCmd int

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

// TODO: the gcode runner should be attached to the gcode itself,
// and created and destroyed as/when new gcode files are loaded, to
// fix the issue of ownership of the gcode
func (a *App) GcodeRunner(ch chan RunnerCmd) {
	running := false
	waiting := 0

	respChan := make(chan string)

	for {
		sendLine := false

		select {
		case cmd := <-ch:
			switch cmd {
			case CmdStart:
				running = true
				a.CycleStart()

			case CmdStop:
				running = false
				// TODO: send a feed hold now, and only send the soft reset once the status is "Hold:2" or whatever
				a.SoftReset()

			case CmdPause:
				running = false
				a.FeedHold()

			case CmdDrain:
				// drain the planner buffer by not sending any more lines, but CycleStart
				running = false
				a.CycleStart()

			case CmdSingle:
				sendLine = true
				running = false
				a.CycleStart()

			}

		case resp := <-respChan:
			// TODO: detect errors and react accordingly?
			_ = resp
			waiting--
		}

		if sendLine || (running && waiting == 0) {
			if a.nextLine < len(a.gcode) {
				line := a.gcode[a.nextLine]
				a.nextLine += 1

				fmt.Printf("> [%s]\n", line)
				// TODO: use the character-counting method instead of waiting for a response?
				if a.g.Command(line, respChan) {
					waiting++
				}

				// TODO: stop requesting G codes after every command (but
				// how else do we display up-to-date G codes?)
				a.g.RequestGCodes()
			} else {
				if a.mode == ModeRun {
					a.PopMode()
				}
			}
		}
	}
}

func (a *App) CycleStart() {
	a.g.CommandRealtime('~')
}

func (a *App) SoftReset() {
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
