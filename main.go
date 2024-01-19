package main

import (
	"fmt"
	"os"

	"gioui.org/app"
	"gioui.org/layout"

	"go.bug.st/serial"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func main() {
	listSerial()
	mode := &serial.Mode{BaudRate: 115200}
	port, err := serial.Open("/dev/ttyUSB2", mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open /dev/ttyUSB2: %v\n", err)
		os.Exit(1)
	}
	a := NewApp()
	a.Connect(NewGrbl(port))
	go a.Run()
	app.Main()
}
