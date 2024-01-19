package main

import (
	"gioui.org/app"
	"gioui.org/layout"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func main() {
	a := NewApp()
	go a.AutoConnect()
	go a.Run()
	app.Main()
}
