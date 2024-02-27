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
	CmdOptionalStop
)

type RunnerCmd int

type GCodeRunner struct {
	app      *App
	gcode    []string
	nextLine int

	running      bool
	stopping     bool
	optionalStop bool
}

func NewGCodeRunner(app *App) *GCodeRunner {
	return &GCodeRunner{app: app}
}

func (r *GCodeRunner) Load(reader io.Reader) {
	gcode := make([]string, 0)

	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		gcode = append(gcode, line)
	}

	r.gcode = gcode
	r.nextLine = 0

	r.app.tp.path.SetGCode(r.Path())
}

func (r *GCodeRunner) Run(ch chan RunnerCmd) {
	r.running = false
	r.optionalStop = true

	waiting := 0

	respChan := make(chan string)

	for {
		sendLine := false

		select {
		case cmd := <-ch:
			switch cmd {
			case CmdStart:
				// start running gcode
				r.running = true
				if r.nextLine > len(r.gcode) {
					// reset to start if run was previously completed
					r.nextLine = 0
				}
				r.CycleStart()

			case CmdStop:
				// send a feed hold now, and a soft-reset once the status is "Hold:0"
				r.running = false
				r.stopping = true
				r.FeedHold()

			case CmdPause:
				// feed hold
				r.running = false
				r.FeedHold()

			case CmdDrain:
				// drain the planner buffer by not sending any more lines, but CycleStart
				r.running = false
				r.CycleStart()

			case CmdSingle:
				// force send a single line only
				sendLine = true
				r.running = false
				r.CycleStart()

			case CmdOptionalStop:
				// toggle optional stopping
				r.optionalStop = !r.optionalStop
			}

		case resp := <-respChan:
			if resp != "ok" {
				r.running = false
				r.FeedHold()
			}
			waiting--
		}

		if r.stopping && r.app.gs.Status == "Hold:0" {
			r.SoftReset()
			r.stopping = false
			r.running = false
			r.nextLine = 0
		}

		if sendLine || (r.running && waiting == 0) {
			if r.nextLine < len(r.gcode) {
				line := r.gcode[r.nextLine]
				r.nextLine += 1

				if r.optionalStop && line == "M1" {
					// turn M1 into M0 if optionalStop
					line = "M0"
				}

				fmt.Printf("> [%s]\n", line)
				if r.app.g.Command(line, respChan) {
					waiting++
				}

				r.app.g.RequestGCodes()
			} else {
				// program is complete
				r.running = false

				if r.app.mode == ModeRun {
					r.app.PopMode()
				}
			}
		}
	}
}

func (r *GCodeRunner) CycleStart() {
	r.app.g.CommandRealtime('~')
}

func (r *GCodeRunner) SoftReset() {
	r.app.g.CommandRealtime(0x18)
	r.app.g.AbortCommands()
}

func (r *GCodeRunner) FeedHold() {
	r.app.g.CommandRealtime('!')
}

func (r *GCodeRunner) Path() []V4d {
	pos := V4d{}

	path := make([]V4d, 0)

	for _, str := range r.gcode {
		line, err := gcode.ParseLine(str)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing gcode line: [%s]: %s, ignoring\n", str, err)
			continue
		}
		G := -1
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
		if G == 0 || G == 1 || G == 2 || G == 3 {
			path = append(path, pos)
		}
	}

	return path
}
