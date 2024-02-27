package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/256dpi/gcode"
)

type GrblSim struct {
	Has4thAxis bool

	s GrblStatus

	planner [15]string

	readBuf  []byte
	writeBuf []byte

	in  chan []byte
	out chan []byte
}

func NewGrblSim() *GrblSim {
	g := &GrblSim{
		in:  make(chan []byte, 3),
		out: make(chan []byte, 3),
	}

	g.s.Status = "Idle"
	g.s.PlannerFree = 15
	g.s.SerialFree = 128
	g.s.GCodes = "G0 G54 G17 G21 G90 G94 M5 M9 T0 F0 S0"
	g.s.FeedOverride = 100
	g.s.SpindleOverride = 100
	g.s.RapidOverride = 100

	return g
}

func (g *GrblSim) Read(p []byte) (int, error) {
	if len(g.readBuf) == 0 {
		r, ok := <-g.out
		if !ok {
			return 0, io.EOF // is EOF right?
		}
		g.readBuf = r
	}

	sz := len(g.readBuf)
	if len(p) < sz {
		sz = len(p)
	}
	copy(p, g.readBuf[:sz])
	g.readBuf = g.readBuf[sz:]

	return sz, nil
}

func (g *GrblSim) Write(p []byte) (int, error) {
	g.in <- p
	return len(p), nil
}

func (g *GrblSim) Close() error {
	close(g.in)
	return nil
}

// run this in a separate goroutine:
func (g *GrblSim) Run() {
	fmt.Println(g.s.String())

	buf := make([]byte, 0)

	for data := range g.in {
		fmt.Printf("[read %s]\n", data)
		for _, ch := range data {
			if ch == '\n' {
				g.processLine(string(buf))
				buf = buf[:0]
			} else if ch == '?' {
				g.statusReport()
			} else if ch == '!' {
				g.feedHold()
			} else if ch == '~' {
				g.cycleStart()
			} else if ch == 0x18 {
				g.softReset()
			} else {
				buf = append(buf, ch)
			}
		}
	}
	close(g.out)
}

func (g *GrblSim) processLine(line string) {
	fmt.Println("> " + line)
	if strings.HasPrefix(line, "$") {
		if line == "$G" {
			g.reply("[GC:" + g.s.GCodes + "]")
			g.reply("ok")
		} else if line == "$$" {
			// TODO: send config
			g.reply("ok")
		} else if strings.HasPrefix(line, "$J=") {
			// TODO: parse + jog
			g.reply("ok")
		} else {
			g.reply("error:2")
		}
	} else {
		gc, err := gcode.ParseLine(line)
		if err != nil {
			g.reply("error:2")
			return
		}
		G := -1
		pos := g.s.Wpos
		for _, code := range gc.Codes {
			if code.Letter == "G" {
				G = int(code.Value)
			} else if code.Letter == "X" {
				pos.X = code.Value
			} else if code.Letter == "Y" {
				pos.Y = code.Value
			} else if code.Letter == "Z" {
				pos.Z = code.Value
			} else if code.Letter == "A" {
				pos.A = code.Value
			}
		}
		if G == 0 || G == 1 || G == 2 || G == 3 {
			g.s.Wpos = pos
		}
		g.reply("ok")
	}
}

func (g *GrblSim) reply(line string) {
	fmt.Println("< " + line)
	g.out <- []byte(line + "\n")
}

func (g *GrblSim) statusReport() {
	fmt.Println(g.s.String())
	g.reply(g.s.String())
}

func (g *GrblSim) feedHold() {
}

func (g *GrblSim) cycleStart() {
}

func (g *GrblSim) softReset() {
}
