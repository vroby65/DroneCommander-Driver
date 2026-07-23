package main

import (
	_ "embed"
	"flag"
	"os"
	"path/filepath"
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
	logPath := flag.String("log", defaultFlightLogPath(), "file persistente del registro di volo")
	flag.Parse()
	desktopui.Run(session.Options{
		CommandAddress: *telloAddress,
		StateAddress:   *stateAddress,
		CommandTimeout: 8 * time.Second,
		LogPath:        *logPath,
	}, fyne.NewStaticResource("Icon.png", iconData))
}

func defaultFlightLogPath() string {
	directory, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "drone-commander-driver", "flight.log")
}
