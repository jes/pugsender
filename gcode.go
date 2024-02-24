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
	CmdOptionalStopEnable
	CmdOptionalStopDisable
)

type RunnerCmd int

type GCodeRunner struct {
	app      *App
	gcode    []string
	nextLine int

	running      bool
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

// TODO: the gcode runner should be attached to the gcode itself,
// and created and destroyed as/when new gcode files are loaded, to
// fix the issue of ownership of the gcode
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
				r.running = true
				r.CycleStart()

			case CmdStop:
				r.running = false
				// TODO: send a feed hold now, and only send the soft reset once the status is "Hold:2" or whatever
				r.SoftReset()

			case CmdPause:
				r.running = false
				r.FeedHold()

			case CmdDrain:
				// drain the planner buffer by not sending any more lines, but CycleStart
				r.running = false
				r.CycleStart()

			case CmdSingle:
				sendLine = true
				r.running = false
				r.CycleStart()

			case CmdOptionalStopEnable:
				r.optionalStop = true

			case CmdOptionalStopDisable:
				r.optionalStop = false
			}

		case resp := <-respChan:
			// TODO: detect errors and react accordingly?
			_ = resp
			waiting--
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
				// TODO: use the character-counting method instead of waiting for a response?
				if r.app.g.Command(line, respChan) {
					waiting++
				}

				// TODO: stop requesting G codes after every command (but
				// how else do we display up-to-date G codes?)
				r.app.g.RequestGCodes()
			} else {
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
	r.app.g.AbortCommands()
	r.app.g.CommandRealtime(0x18)
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
