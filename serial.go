package main

import (
	"fmt"
	"os"

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
