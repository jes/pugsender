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
	a.ReadConf()
	go a.AutoConnect()
	go a.Run()
	app.Main()
}
