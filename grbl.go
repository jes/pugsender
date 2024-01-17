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
	Closed          bool
	Status          string
	Wco             [4]float32
	Mpos            [4]float32
	Wpos            [4]float32
	PlannerSize     int
	PlannerFree     int
	Spindle         bool
	SpindleCw       bool
	SpindleSpeed    bool
	FloodCoolant    bool
	MistCoolant     bool
	FeedOverride    float32
	RapidOverride   float32
	SpindleOverride float32
	FeedRate        float32
	StatusUpdate    chan struct{}
}

func NewGrbl(port io.ReadWriteCloser) *Grbl {
	return &Grbl{
		SerialPort: port,
	}
}

// implements io.Writer
func (g *Grbl) Write(p []byte) (n int, err error) {
	return g.SerialPort.Write(p)
}

// implements io.Closer
func (g *Grbl) Close() error {
	g.Closed = true
	return g.SerialPort.Close()
}

func (g *Grbl) Monitor() {
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
				fmt.Fprintf(os.Stderr, "error asking for status update, ignoring: %v", err)
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
			status := strings.Trim(line, "<>")
			parts := strings.Split(status, "|")
			g.Status = parts[0]
		}
	}
	g.Closed = true
}
