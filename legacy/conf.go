package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) WriteConf(gs GrblStatus) {
	filename := a.ConfFile()
	f, err := os.Create(a.ConfFile())
	if err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", filename, err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "wpos=%.3f,%.3f,%.3f,%.3f\n", gs.Wpos.X, gs.Wpos.Y, gs.Wpos.Z, gs.Wpos.A)
}

func (a *App) ReadConf() {
	filename := a.ConfFile()
	f, err := os.Open(a.ConfFile())
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", filename, err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		val := parts[1]
		valv4d, _, _ := ParseV4d(val)

		if key == "wpos" {
			a.g.SetWpos(valv4d)
			// XXX: assigning to gsNew is kind of a bodge, but we want this so
			// that when the application first loads up and is not yet connected
			// to grbl, it shows the saved coordinates
			a.gsNew.Wpos = valv4d
			a.gsNew.Wco = a.gs.Mpos.Sub(valv4d)
		} else {
			fmt.Fprintf(os.Stderr, "%s: unrecognised config key: [%s]\n", filename, key)
		}
	}
}

func (a *App) ConfFile() string {
	confdir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "os.UserConfigDir: %v; reverting to '.'\n", err)
		confdir = "."
	}
	dir := filepath.Join(confdir, "pugsender")
	os.MkdirAll(dir, os.ModePerm)
	return filepath.Join(dir, "pugsender.conf")
}
