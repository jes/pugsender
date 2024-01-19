package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Grbl struct {
	SerialPort      io.ReadWriteCloser
	PortName        string
	Closed          bool
	Status          string
	Wco             V4d
	Mpos            V4d
	Wpos            V4d
	Dtg             V4d // TODO: how can we calculate this?
	Vel             V4d
	PlannerSize     int
	PlannerFree     int
	SerialSize      int
	SerialFree      int
	SpindleCw       bool
	SpindleCcw      bool
	FloodCoolant    bool
	MistCoolant     bool
	FeedOverride    float64
	RapidOverride   float64
	SpindleOverride float64
	FeedRate        float64
	SpindleSpeed    float64
	Probe           bool
	StatusUpdate    chan struct{}
	UpdateTime      time.Time
}

func NewGrbl(port io.ReadWriteCloser, portName string) *Grbl {
	g := &Grbl{
		SerialPort:   port,
		PortName:     portName,
		StatusUpdate: make(chan struct{}),
	}
	if port == nil {
		g.Closed = true
	}
	return g
}

// implements io.Writer
func (g *Grbl) Write(p []byte) (n int, err error) {
	// TODO: is there a race condition where concurrent writes can end up interleaved?
	os.Stdout.Write(p)
	return g.SerialPort.Write(p)
}

// implements io.Closer
func (g *Grbl) Close() error {
	g.Closed = true
	var err error
	if g.SerialPort != nil {
		err = g.SerialPort.Close()
	}
	g.StatusUpdate <- struct{}{}
	return err
}

func (g *Grbl) Monitor() {
	if g.SerialPort == nil {
		g.Close()
	}

	// ask for a status update every 200ms, until Closed
	ticker := time.NewTicker(200 * time.Millisecond)
	go func() {
		for {
			<-ticker.C
			if g.Closed {
				return
			}
			_, err := g.Write([]byte{'?'})
			if err != nil {
				fmt.Fprintf(os.Stderr, "error asking for status update, closing: %v", err)
				g.Close()
				return
			}
		}
	}()

	// read from the serial port
	// send a struct{} to the StatusUpdate channel whenever there isa new status report
	scanner := bufio.NewScanner(g.SerialPort)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(scanner.Text())
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			// status update
			g.ParseStatus(line)
		}
	}
	g.Close()
}

// "status" should be a status report line from Grbl
func (g *Grbl) ParseStatus(status string) {
	prevWpos := g.Wpos
	prevUpdateTime := g.UpdateTime

	status = strings.Trim(status, "<>")
	parts := strings.Split(status, "|")
	g.Status = parts[0]

	// grbl in theory should give us either a wpos or an mpos
	// every time, but track them separately just in case
	givenWpos := false
	givenMpos := false

	for _, part := range parts[1:] {
		keyval := strings.SplitN(part, ":", 2)
		if len(keyval) != 2 {
			fmt.Fprintf(os.Stderr, "unrecognised status item [%s]\n", part)
			continue
		}
		key := keyval[0]
		keylc := strings.ToLower(key)
		val := keyval[1]
		valv4d, _ := ParseV4d(val)

		if keylc == "wpos" { // work position
			givenWpos = true
			g.Wpos = valv4d
		} else if keylc == "mpos" { // machine position
			givenMpos = true
			g.Mpos = valv4d
		} else if keylc == "wco" { // work coordinate offset
			g.Wco = valv4d
		} else if keylc == "ov" { // overrides
			g.FeedOverride = valv4d.X
			g.RapidOverride = valv4d.X
			g.SpindleOverride = valv4d.X
		} else if keylc == "a" { // accessories
			g.SpindleCw = strings.Contains(val, "S")
			g.SpindleCcw = strings.Contains(val, "C")
			g.FloodCoolant = strings.Contains(val, "F")
			g.MistCoolant = strings.Contains(val, "M")
		} else if keylc == "bf" { // buffers
			g.PlannerFree = int(valv4d.X)
			g.SerialFree = int(valv4d.Y)
			if g.PlannerFree > g.PlannerSize {
				g.PlannerSize = g.PlannerFree
			}
			if g.SerialFree > g.SerialSize {
				g.SerialSize = g.SerialFree
			}
		} else if keylc == "fs" { // feed/speed
			g.FeedRate = valv4d.X
			g.SpindleSpeed = valv4d.Y
		} else if keylc == "f" { // feed rate
			g.FeedRate = valv4d.X
		} else if keylc == "pn" { // pins
			g.Probe = strings.Contains(val, "P")
			// XXX: when the probe is deactivated, grbl doesn't tell us, see https://github.com/gnea/grbl/issues/1242
		} else {
			fmt.Fprintf(os.Stderr, "unrecognised field: %s\n", key)
		}
	}

	if givenMpos {
		g.Wpos = g.Mpos.Add(g.Wco)
	} else if givenWpos {
		g.Mpos = g.Wpos.Sub(g.Wco)
	}

	g.UpdateTime = time.Now()

	distanceMoved := g.Wpos.Sub(prevWpos)
	g.Vel = distanceMoved.Div(g.UpdateTime.Sub(prevUpdateTime).Seconds())

	g.StatusUpdate <- struct{}{}
}
