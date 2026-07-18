package main

import (
	_ "embed"
	"flag"
	"time"

	"fyne.io/fyne/v2"

	"github.com/vroby65/DroneCommander-Driver/desktopui"
	"github.com/vroby65/DroneCommander-Driver/session"
)

//go:embed Icon.png
var iconData []byte

func main() {
	telloAddress := flag.String("tello", "192.168.10.1:8889", "indirizzo UDP comandi Tello")
	stateAddress := flag.String("state", ":8890", "indirizzo locale telemetria Tello")
	flag.Parse()
	desktopui.Run(session.Options{
		CommandAddress: *telloAddress,
		StateAddress:   *stateAddress,
		CommandTimeout: 8 * time.Second,
	}, fyne.NewStaticResource("Icon.png", iconData))
}
