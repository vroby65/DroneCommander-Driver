// Package desktopui provides the native Fyne user interface.
package desktopui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/vroby65/DroneCommander-Driver/session"
)

type UI struct {
	app     fyne.App
	window  fyne.Window
	session *session.Session

	connection       *widget.Label
	battery          *widget.Label
	height           *widget.Label
	yaw              *widget.Label
	flightTime       *widget.Label
	programName      *widget.Label
	programMeta      *widget.Label
	warnings         *widget.Label
	lastError        *widget.Label
	logs             *widget.Entry
	simulation       *widget.Check
	minimumBattery   *widget.Entry
	autoLand         *widget.Check
	loadButton       *widget.Button
	connectButton    *widget.Button
	disconnectButton *widget.Button
	runButton        *widget.Button
	stopButton       *widget.Button
	landButton       *widget.Button
	emergencyButton  *widget.Button
	lastLogText      string
	stopRefresh      chan struct{}
}

func Run(options session.Options, icon fyne.Resource) {
	nativeApp := app.NewWithID("org.dronecommander.tello-driver")
	nativeApp.Settings().SetTheme(theme.DarkTheme())
	if icon != nil {
		nativeApp.SetIcon(icon)
	}
	ui := &UI{app: nativeApp, window: nativeApp.NewWindow("Drone Commander · Tello Driver"), session: session.New(options), stopRefresh: make(chan struct{})}
	ui.build()
	ui.bindKeyboard()
	ui.window.Resize(fyne.NewSize(920, 760))
	ui.window.CenterOnScreen()
	ui.window.SetCloseIntercept(ui.close)
	ui.refresh(ui.session.Snapshot())
	go ui.refreshLoop()
	ui.window.ShowAndRun()
}

func (u *UI) build() {
	title := widget.NewLabelWithStyle("Tello Driver", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel("Drone Commander · controllo indoor nativo")
	u.connection = widget.NewLabelWithStyle("● Non connesso", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	header := container.NewBorder(nil, nil, container.NewVBox(title, subtitle), u.connection)
	u.battery = metric("— %")
	u.height = metric("— cm")
	u.yaw = metric("—°")
	u.flightTime = metric("— s")
	telemetry := widget.NewCard("Telemetria", "Aggiornamento diretto dal Tello", container.NewGridWithColumns(4, metricBox("BATTERIA", u.battery), metricBox("QUOTA", u.height), metricBox("ROTTA", u.yaw), metricBox("TEMPO VOLO", u.flightTime)))

	u.programName = widget.NewLabelWithStyle("Nessun programma selezionato", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.programName.Wrapping = fyne.TextWrapWord
	u.programMeta = widget.NewLabel("Carica un workspace XML esportato da Drone Commander.")
	u.warnings = widget.NewLabel("")
	u.warnings.Wrapping = fyne.TextWrapWord
	u.loadButton = widget.NewButtonWithIcon("Scegli XML", theme.FolderOpenIcon(), u.chooseProgram)
	programRow := container.NewBorder(nil, nil, nil, u.loadButton, container.NewVBox(u.programName, u.programMeta, u.warnings))
	programCard := widget.NewCard("1 · Programma", "Il file viene analizzato prima di abilitare il volo.", programRow)

	u.simulation = widget.NewCheck("Modalita simulazione (nessun pacchetto al drone)", nil)
	u.connectButton = widget.NewButtonWithIcon("Connetti", theme.RadioButtonCheckedIcon(), u.connect)
	u.connectButton.Importance = widget.HighImportance
	u.disconnectButton = widget.NewButton("Disconnetti", u.disconnect)
	connectionButtons := container.NewHBox(u.connectButton, u.disconnectButton)
	connectionCard := widget.NewCard("2 · Connessione", "Per il volo reale collega prima il computer alla rete Wi-Fi TELLO-…", container.NewVBox(u.simulation, connectionButtons))

	u.minimumBattery = widget.NewEntry()
	u.minimumBattery.SetText("20")
	u.minimumBattery.SetPlaceHolder("20")
	u.minimumBattery.Validator = func(value string) error {
		number, err := strconv.Atoi(value)
		if err != nil || number < 0 || number > 100 {
			return fmt.Errorf("usa un valore tra 0 e 100")
		}
		return nil
	}
	u.autoLand = widget.NewCheck("Atterra automaticamente al termine", nil)
	u.autoLand.SetChecked(true)
	unitNotice := widget.NewLabelWithStyle("Scala indoor fissa: 1 unita = 1 cm", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	unitHelp := widget.NewLabel("Il Tello accetta spostamenti di almeno 20 cm: usa valori ≥ 20 per i movimenti su un solo asse.")
	unitHelp.Wrapping = fyne.TextWrapWord
	batteryRow := container.NewBorder(nil, nil, widget.NewLabel("Batteria minima (%)"), nil, u.minimumBattery)
	settings := container.NewVBox(unitNotice, unitHelp, batteryRow, u.autoLand)

	u.runButton = widget.NewButtonWithIcon("Avvia programma", theme.MediaPlayIcon(), u.runProgram)
	u.runButton.Importance = widget.HighImportance
	u.stopButton = widget.NewButtonWithIcon("Stop / hovering", theme.MediaStopIcon(), func() { u.safety("stop") })
	u.landButton = widget.NewButtonWithIcon("Atterra", theme.MoveDownIcon(), func() { u.safety("land") })
	u.emergencyButton = widget.NewButtonWithIcon("ARRESTO MOTORI", theme.WarningIcon(), u.confirmEmergency)
	u.emergencyButton.Importance = widget.DangerImportance
	controls := container.NewGridWithColumns(4, u.runButton, u.stopButton, u.landButton, u.emergencyButton)
	safetyText := widget.NewLabel("Arresto motori e un comando di emergenza: il drone cade immediatamente.")
	safetyText.Wrapping = fyne.TextWrapWord
	flightCard := widget.NewCard("3 · Volo", "Verifica il percorso in simulazione e libera l'area indoor.", container.NewVBox(settings, widget.NewSeparator(), controls, safetyText))

	u.lastError = widget.NewLabel("")
	u.lastError.Wrapping = fyne.TextWrapWord
	u.logs = widget.NewMultiLineEntry()
	u.logs.Disable()
	u.logs.SetMinRowsVisible(9)
	logCard := widget.NewCard("Registro di volo", "Comandi SDK e risposte del drone", u.logs)
	content := container.NewVBox(telemetry, programCard, connectionCard, flightCard, u.lastError, logCard)
	scroll := container.NewVScroll(content)
	u.window.SetContent(container.NewBorder(container.NewPadded(header), nil, nil, nil, scroll))
}

func metric(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
}
func metricBox(title string, value *widget.Label) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{}), value)
}

func (u *UI) chooseProgram() {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			u.showError(err)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()
		data, readErr := io.ReadAll(io.LimitReader(reader, 2<<20))
		if readErr != nil {
			u.showError(readErr)
			return
		}
		if loadErr := u.session.LoadProgram(reader.URI().Name(), data); loadErr != nil {
			u.showError(loadErr)
			return
		}
		u.refresh(u.session.Snapshot())
	}, u.window)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".xml"}))
	fileDialog.Resize(fyne.NewSize(760, 520))
	fileDialog.Show()
}

func (u *UI) connect() {
	u.connectButton.Disable()
	simulation := u.simulation.Checked
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := u.session.Connect(ctx, simulation); err != nil {
			fyne.Do(func() { u.showError(err) })
		}
		fyne.Do(func() { u.refresh(u.session.Snapshot()) })
	}()
}
func (u *UI) disconnect() {
	go func() {
		if err := u.session.Disconnect(); err != nil {
			fyne.Do(func() { u.showError(err) })
		}
		fyne.Do(func() { u.refresh(u.session.Snapshot()) })
	}()
}
func (u *UI) runProgram() {
	if err := u.minimumBattery.Validate(); err != nil {
		u.showError(err)
		return
	}
	battery, _ := strconv.Atoi(u.minimumBattery.Text)
	if err := u.session.Start(session.RunConfig{MinimumBattery: battery, AutoLand: u.autoLand.Checked}); err != nil {
		u.showError(err)
		return
	}
	u.refresh(u.session.Snapshot())
}
func (u *UI) safety(command string) {
	go func() {
		if err := u.session.Safety(command); err != nil {
			fyne.Do(func() { u.showError(err) })
		}
		fyne.Do(func() { u.refresh(u.session.Snapshot()) })
	}()
}
func (u *UI) confirmEmergency() {
	dialog.ShowConfirm("Arresto immediato dei motori", "Il Tello cadra immediatamente. Inviare davvero EMERGENCY?", func(confirmed bool) {
		if confirmed {
			u.safety("emergency")
		}
	}, u.window)
}
func (u *UI) showError(err error) { dialog.ShowError(err, u.window) }

func (u *UI) bindKeyboard() {
	canvas, ok := u.window.Canvas().(desktop.Canvas)
	if !ok {
		return
	}
	canvas.SetOnKeyDown(func(event *fyne.KeyEvent) { u.session.SetKey(string(event.Name), true) })
	canvas.SetOnKeyUp(func(event *fyne.KeyEvent) { u.session.SetKey(string(event.Name), false) })
}
func (u *UI) refreshLoop() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			snapshot := u.session.Snapshot()
			fyne.Do(func() { u.refresh(snapshot) })
		case <-u.stopRefresh:
			return
		}
	}
}

func (u *UI) refresh(snapshot session.Snapshot) {
	if snapshot.Connected {
		if snapshot.Simulated {
			u.connection.SetText("● Simulazione")
		} else {
			u.connection.SetText("● Tello connesso")
		}
		u.battery.SetText(fmt.Sprintf("%d %%", snapshot.Telemetry.Battery))
		u.height.SetText(fmt.Sprintf("%d cm", snapshot.Telemetry.Height))
		u.yaw.SetText(fmt.Sprintf("%d°", snapshot.Telemetry.Yaw))
		u.flightTime.SetText(fmt.Sprintf("%d s", snapshot.Telemetry.FlightTime))
	} else {
		u.connection.SetText("● Non connesso")
		u.battery.SetText("— %")
		u.height.SetText("— cm")
		u.yaw.SetText("—°")
		u.flightTime.SetText("— s")
	}
	if snapshot.Summary != nil {
		u.programName.SetText(snapshot.ProgramName)
		u.programMeta.SetText(fmt.Sprintf("%d blocchi · %d comandi drone", snapshot.Summary.Blocks, snapshot.Summary.Commands))
		u.warnings.SetText(strings.Join(snapshot.Summary.Warnings, " "))
	} else {
		u.programName.SetText("Nessun programma selezionato")
		u.programMeta.SetText("Carica un workspace XML esportato da Drone Commander.")
		u.warnings.SetText("")
	}
	u.lastError.SetText(snapshot.LastError)
	var builder strings.Builder
	for _, entry := range snapshot.Logs {
		fmt.Fprintf(&builder, "[%s] %s\n", entry.Time, entry.Message)
	}
	logText := builder.String()
	if logText != u.lastLogText {
		u.logs.SetText(logText)
		u.lastLogText = logText
		u.logs.CursorRow = len(snapshot.Logs)
		u.logs.Refresh()
	}
	setEnabled(u.loadButton, !snapshot.Running && !snapshot.Connecting)
	setEnabled(u.connectButton, !snapshot.Connected && !snapshot.Running && !snapshot.Connecting)
	setEnabled(u.disconnectButton, snapshot.Connected && !snapshot.Running)
	setEnabled(u.runButton, snapshot.Connected && snapshot.Summary != nil && !snapshot.Running)
	setEnabled(u.stopButton, snapshot.Connected && snapshot.Running)
	setEnabled(u.landButton, snapshot.Connected)
	setEnabled(u.emergencyButton, snapshot.Connected)
	if snapshot.Connected || snapshot.Running || snapshot.Connecting {
		u.simulation.Disable()
	} else {
		u.simulation.Enable()
	}
}

func setEnabled(button *widget.Button, enabled bool) {
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}
func (u *UI) close() {
	select {
	case <-u.stopRefresh:
	default:
		close(u.stopRefresh)
	}
	_ = u.session.Close()
	u.window.SetCloseIntercept(nil)
	u.window.Close()
}
