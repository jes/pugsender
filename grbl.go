package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Grbl struct {
	serialPort    io.ReadWriteCloser
	status        GrblStatus
	writeChan     chan GrblResponse
	abortChan     chan struct{}
	responseQueue []GrblResponse
}

type GrblResponse struct {
	responseChan chan string
	command      string
}

func NewGrbl(port io.ReadWriteCloser, portName string) *Grbl {
	status := DefaultGrblStatus()
	if port == nil {
		status.Status = "Disconnected"
	} else {
		status.Closed = false
	}
	status.PortName = portName
	g := &Grbl{
		serialPort: port,
		status:     status,
		writeChan:  make(chan GrblResponse, 10),
		abortChan:  make(chan struct{}),
	}
	return g
}

// add the given line to the command queue, sending the response
// to the given channel, and return true,
// or return false if the command was not sent
//
// only use this function for commands that expect a response,
// use CommandRealtime() for commands that give no response
func (g *Grbl) Command(line string, respChan chan string) bool {
	if !g.status.Ready {
		return false
	}

	// canonicalise line ending
	line = strings.TrimSpace(line) + "\n"

	// not enough space in Grbl's input buffer? reject the command
	// +1 because we need to leave at least 1 byte free else Grbl locks up
	if g.status.SerialFree <= len(line)+1 {
		fmt.Fprintf(os.Stderr, "not running command because serial is full: %s\n", line)
		return false
	}

	g.status.SerialFree -= len(line)
	g.writeChan <- GrblResponse{respChan, line}

	return true
}

// add the given line to the command queue, return true if
// successful or false if not
//
// spawn a goroutine to consume and ignore the response
func (g *Grbl) CommandIgnore(line string) bool {
	if !g.status.Ready {
		return false
	}
	c := make(chan string)
	go func() { <-c }() // ignore response
	if !g.Command(line, c) {
		return false
	}
	return true
}

// add the given line to the command queue, return
// (true, "...response...") if successful or (false, "") if not
//
// block until the response is received
func (g *Grbl) CommandWait(line string) (bool, string) {
	if !g.status.Ready {
		return false, ""
	}
	c := make(chan string, 1)
	if !g.Command(line, c) {
		return false, ""
	}
	resp := <-c
	return true, resp
}

// send the given realtime command, return true if successful
// or false if not
func (g *Grbl) CommandRealtime(cmd byte) bool {
	if g.status.Closed {
		return false
	}
	g.writeChan <- GrblResponse{nil, string(cmd)}
	return true
}

// implements io.Closer
func (g *Grbl) Close() error {
	if g.status.Closed {
		return nil
	}
	g.status.Closed = true
	g.status.Ready = false
	g.status.Status = "Disconnected"
	var err error
	if g.serialPort != nil {
		err = g.serialPort.Close()
	}
	return err
}

func (g *Grbl) Monitor(statusUpdate chan GrblStatus) {
	if statusUpdate != nil {
		defer close(statusUpdate)
	}

	if g.serialPort == nil {
		g.Close()
		return
	}
	defer g.Close()

	// ask for a status update every 200ms, until Closed
	//
	// "We recommend querying Grbl for a ? real-time status report
	// at no more than 5Hz. 10Hz may be possible, but at some point,
	// there are diminishing returns and you are taxing Grbl's CPU
	// more by asking it to generate and send a lot of position
	// data."
	// https://github.com/grbl/grbl/wiki/Interfacing-with-Grbl
	g.RequestStatusUpdate()
	statusTicker := time.NewTicker(200 * time.Millisecond)
	defer statusTicker.Stop()

	// ask for active g-codes every second, until closed
	gcodesTicker := time.NewTicker(time.Second)
	defer gcodesTicker.Stop()

	// make a regex for matching config lines (like "$120=25.000")
	configRe := regexp.MustCompile("^\\$(\\d+)=(-?[0-9\\.]+)$")

	readChan := make(chan string)
	go g.readSerial(readChan)

loop:
	for {
		select {
		case <-statusTicker.C: // request a status update
			if !g.RequestStatusUpdate() {
				break loop
			}

		case <-gcodesTicker.C: // request G codes
			if !g.RequestGCodes() {
				break loop
			}

		case <-g.abortChan: // abort commands
			g.doAbortCommands()

		case r := <-g.writeChan: // write to grbl
			if g.status.SerialFree < len(r.command) {
				fmt.Fprintf(os.Stderr, "(in writechan) not running command because serial is full: %s\n", r.command)
				if r.responseChan != nil {
					r.responseChan <- "fail:buffer full"
				}
				continue loop
			}

			if r.responseChan != nil {
				// responseChan is nil for commands that don't expect
				// a response (i.e. realtime commands)
				g.responseQueue = append(g.responseQueue, r)
			}

			_, err := g.serialPort.Write([]byte(r.command))
			if err != nil {
				// don't send a response if no responseChan, else the response queue will be out of sync
				if r.responseChan != nil {
					g.SendResponse(fmt.Sprintf("fail:write error: %v", err))
				}
				break loop
			}

		case line := <-readChan: // read from grbl
			if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
				// status update
				g.ParseStatus(line, statusUpdate)
			} else if strings.HasPrefix(line, "[GC:") {
				// g-codes update
				g.ParseGCodes(line)
			} else if configRe.MatchString(line) {
				// config value ("$120=25.000")
				vals := configRe.FindStringSubmatch(line)
				key, err := strconv.ParseInt(vals[1], 10, 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: strconv.ParseInt(%s): %v\n", line, vals[1], err)
					continue loop
				}
				val, err := strconv.ParseFloat(vals[2], 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: strconv.ParseFloat(%s): %v\n", line, vals[2], err)
					continue loop
				}
				g.status.GrblConfig[int(key)] = val
			} else if strings.HasPrefix(line, "ok") || strings.HasPrefix(line, "error") {
				g.SendResponse(line)
			}
		}
	}
}

// read lines from the serial port and put then on channel c
func (g *Grbl) readSerial(c chan string) {
	scanner := bufio.NewScanner(g.serialPort)
	for scanner.Scan() {
		c <- scanner.Text()
	}
	close(c)
}

// request a status update, return true if ok or false if not
func (g *Grbl) RequestStatusUpdate() bool {
	return g.CommandRealtime('?')
}

// request active gcodes, return true if ok or false if not
func (g *Grbl) RequestGCodes() bool {
	if g.status.Closed {
		return false
	}
	if !g.status.Ready {
		return true
	}
	if g.status.WaitingForGCodes {
		// don't have more than one request in-flight at any time
		return true
	}
	g.status.WaitingForGCodes = true
	ok := g.CommandIgnore("$G")
	if !ok && g.status.Closed {
		return false
	}
	return true
}

func (g *Grbl) RequestGrblConfig() bool {
	return g.CommandIgnore("$$")
}

// "status" should be a status report line from Grbl
// send a struct{} to the StatusUpdate channel whenever there isa new status report
func (g *Grbl) ParseStatus(status string, ch chan GrblStatus) {
	g.status.Ready = true

	prevMpos := g.status.Mpos
	prevUpdateTime := g.status.UpdateTime

	status = strings.Trim(status, "<>")
	parts := strings.Split(status, "|")
	g.status.Status = parts[0]

	if g.status.GCodes == "" {
		// at startup, get the active g-codes without having to wait for the timer to fire
		g.RequestGCodes()
	}

	if len(g.status.GrblConfig) == 0 {
		// at startup, grab the grbl config
		g.RequestGrblConfig()
	}

	// grbl in theory should give us either a wpos or an mpos
	// every time, but track them separately just in case
	givenWpos := false
	givenMpos := false

	newProbeState := false
	newPn := ""

	for _, part := range parts[1:] {
		keyval := strings.SplitN(part, ":", 2)
		if len(keyval) != 2 {
			fmt.Fprintf(os.Stderr, "unrecognised status item [%s]\n", part)
			continue
		}
		key := keyval[0]
		keylc := strings.ToLower(key)
		val := keyval[1]
		valv4d, axes, _ := ParseV4d(val)

		if keylc == "wpos" { // work position
			givenWpos = true
			g.status.Wpos = valv4d
		} else if keylc == "mpos" { // machine position
			givenMpos = true
			g.status.Mpos = valv4d
		} else if keylc == "wco" { // work coordinate offset
			g.status.Wco = valv4d
			g.status.Has4thAxis = (axes == 4)
		} else if keylc == "ov" { // overrides
			g.status.FeedOverride = valv4d.X
			g.status.RapidOverride = valv4d.Y
			g.status.SpindleOverride = valv4d.Z
		} else if keylc == "a" { // accessories
			g.status.SpindleCw = strings.Contains(val, "S")
			g.status.SpindleCcw = strings.Contains(val, "C")
			g.status.FloodCoolant = strings.Contains(val, "F")
			g.status.MistCoolant = strings.Contains(val, "M")
		} else if keylc == "bf" { // buffers
			g.status.PlannerFree = int(valv4d.X)
			serialFree := int(valv4d.Y)
			if serialFree != g.status.SerialFree {
				fmt.Fprintf(os.Stderr, "BUG?? serial buffer space out of sync: we thought %d bytes free, but Grbl reports %d\n", g.status.SerialFree, serialFree)
			}
			if g.status.PlannerFree > g.status.PlannerSize {
				g.status.PlannerSize = g.status.PlannerFree
			}
			if serialFree > g.status.SerialSize {
				g.status.SerialSize = serialFree
			}
		} else if keylc == "fs" { // feed/speed
			g.status.FeedRate = valv4d.X
			g.status.SpindleSpeed = valv4d.Y
		} else if keylc == "f" { // feed rate
			g.status.FeedRate = valv4d.X
		} else if keylc == "pn" { // pins
			newProbeState = strings.Contains(val, "P")
			newPn = val
		} else {
			fmt.Fprintf(os.Stderr, "unrecognised field: %s\n", key)
		}
	}

	g.status.Probe = newProbeState
	g.status.Pn = newPn

	if givenMpos {
		g.status.Wpos = g.status.Mpos.Sub(g.status.Wco)
	} else if givenWpos {
		g.status.Mpos = g.status.Wpos.Add(g.status.Wco)
	}

	g.status.UpdateTime = time.Now()

	distanceMoved := g.status.Mpos.Sub(prevMpos)
	g.status.Vel = distanceMoved.Div(g.status.UpdateTime.Sub(prevUpdateTime).Minutes())

	if ch != nil {
		// send a status update unless doing so would block
		select {
		case ch <- g.status:
		default:
		}
	}
}

func (g *Grbl) ParseGCodes(line string) {
	g.status.GCodes = strings.TrimRight(strings.TrimPrefix(line, "[GC:"), "]")
	g.status.WaitingForGCodes = false
}

func (g *Grbl) SendResponse(line string) {
	l := len(g.responseQueue)
	if l == 0 {
		fmt.Fprintf(os.Stderr, "BUG: wanted to send a command response, but no channels are waiting; this means the sender is out of sync: %s\n", line)
		return
	}

	r := g.responseQueue[0]
	g.responseQueue = g.responseQueue[1:]

	if strings.HasPrefix(line, "error") {
		fmt.Printf("[%s]: %s\n", r.command, line)
	}

	g.status.SerialFree += len(r.command)
	r.responseChan <- line
}

func (g *Grbl) AbortCommands() {
	g.abortChan <- struct{}{}
}

func (g *Grbl) doAbortCommands() {
	for _, r := range g.responseQueue {
		r.responseChan <- "fail:aborted"
	}
	g.responseQueue = make([]GrblResponse, 0)
}

func (g *Grbl) SetWpos(p V4d) bool {
	// XXX: uses CommandWait, which can block the main UI thread
	// because of https://github.com/gnea/grbl/wiki/Grbl-v1.1-Interface#eeprom-issues
	// we need to wait until a G10 is acknowledged before proceeding
	if g.status.Status != "Idle" {
		// only allow setting WCO in Idle state
		return false
	}
	line := fmt.Sprintf("G10L20P1X%.3fY%.3fZ%.3f", p.X, p.Y, p.Z)
	if g.status.Has4thAxis {
		line += fmt.Sprintf("A%.3f", p.A)
	}
	ok, _ := g.CommandWait(line)
	return ok
}

func (g *Grbl) SetFeedOverride(v int) bool {
	delta := v - int(g.status.FeedOverride)
	return g.SendOverrideDelta(delta, 0x91, 0x92, 0x93, 0x94)
}

func (g *Grbl) SetRapidOverride(v int) bool {
	if v < 38 {
		// 25% rapid override
		return g.CommandRealtime(0x97)
	} else if v < 68 {
		// 50% rapid override
		return g.CommandRealtime(0x96)
	} else {
		// 100% rapid override
		return g.CommandRealtime(0x95)
	}
}

func (g *Grbl) SetSpindleOverride(v int) bool {
	delta := v - int(g.status.SpindleOverride)
	return g.SendOverrideDelta(delta, 0x9a, 0x9b, 0x9c, 0x9d)
}

func (g *Grbl) SendOverrideDelta(delta int, plus10 byte, minus10 byte, plus1 byte, minus1 byte) bool {
	for delta >= 10 {
		if !g.CommandRealtime(plus10) {
			return false
		}
		delta -= 10
	}
	for delta <= -10 {
		if !g.CommandRealtime(minus10) {
			return false
		}
		delta += 10
	}
	for delta >= 1 {
		if !g.CommandRealtime(plus1) {
			return false
		}
		delta--
	}
	for delta <= -1 {
		if !g.CommandRealtime(minus1) {
			return false
		}
		delta++
	}
	return true
}
