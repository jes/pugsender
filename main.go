package main

import (
	"fmt"
	"os"

	"gioui.org/app"
	"gioui.org/layout"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func usage(rc int) {
	fmt.Fprintf(os.Stderr, `usage: pugsender [option]

options:
        <device>  Connect to Grbl at <device> (e.g. "/dev/ttyUSB0").
	--sim     Use simulator instead of real Grbl hardware.
	--help    Show this help.

Pugsender is a project by James Stanley <james@incoherency.co.uk>.

https://github.com/jes/pugsender
`)
	os.Exit(rc)
}

func main() {
	a := NewApp()
	go a.ReadConf()

	if len(os.Args) > 2 {
		usage(1)
	} else if len(os.Args) == 2 {
		// connect to named port, or sim, if there's an arg
		if os.Args[1] == "--sim" {
			sim := NewGrblSim()
			go sim.Run()
			g := NewGrbl(sim, "<sim>")
			ch := make(chan GrblStatus)
			go g.Monitor(ch)
			a.Connect(g, ch)
		} else if os.Args[1] == "--help" {
			usage(0)
		} else {
			a.TryToConnect(os.Args[1])
		}
	} else {
		// auto-connect if no args
		go a.AutoConnect()
	}

	go a.Run()

	app.Main()
}
