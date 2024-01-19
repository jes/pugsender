package main

import (
	"fmt"
	"os"
	"time"

	"go.bug.st/serial"
)

func listSerial() {
	ports, err := serial.GetPortsList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list serial ports: %v\n", err)
		return
	}

	for _, port := range ports {
		fmt.Printf("port: %s\n", port)
	}
}

func (a *App) AutoConnect() {
	// try to auto-connect every second
	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			<-ticker.C
			// if we already have a connection, or we don't want to auto-connect, do nothing
			if !a.g.Closed || !a.autoConnect {
				continue
			}

			ports, err := serial.GetPortsList()
			if err != nil {
				fmt.Fprintf(os.Stderr, "list serial ports: %v\n", err)
				continue
			}

			for _, port := range ports {
				// TODO: only try to connect to ports that have
				// newly-appeared since we last tried?
				go a.TryToConnect(port)
			}
		}
	}()
}

func (a *App) TryToConnect(port string) {
	fmt.Printf("try to connect to %s\n", port)
	mode := &serial.Mode{BaudRate: 115200}
	file, err := serial.Open(port, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", port, err)
		return
	}
	g := NewGrbl(file, port)
	go g.Monitor()
	select {
	case <-g.StatusUpdate:
		// if this port gave us a successful grbl status update, and we still want auto-connection, use this one
		if !g.Closed && a.g.Closed && a.autoConnect {
			a.Connect(g)
		}
	case <-time.After(time.Second):
		// time out after 1 second
		g.Close()
	}
}
