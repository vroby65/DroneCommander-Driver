// Package desktopui provides the native Fyne user interface.
package desktopui

import (
	"context"
	"errors"
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
	fyneLang "fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/session"
)

type UI struct {
	app     fyne.App
	window  fyne.Window
	session *session.Session
	// programURI is kept so the XML can be read again before every run. The
	// parsed program must not become a stale snapshot when the file is edited.
	programURI fyne.URI
	languageID string

	connection       *widget.Label
	battery          *widget.Label
	height           *widget.Label
	yaw              *widget.Label
	flightTime       *widget.Label
	programName      *widget.Label
	programMeta      *widget.Label
	warnings         *widget.Label
	lastError        *widget.Label
	logs             *widget.RichText
	logScroll        *container.Scroll
	language         *widget.Select
	simulation       *widget.Check
	minimumBattery   *widget.Entry
	autoLand         *widget.Check
	loadButton       *widget.Button
	editButton       *widget.Button
	connectButton    *widget.Button
	disconnectButton *widget.Button
	runButton        *widget.Button
	stopButton       *widget.Button
	landButton       *widget.Button
	emergencyButton  *widget.Button
	preferredSize    fyne.Size
	lastLogText      string
	stopRefresh      chan struct{}
}

func Run(options session.Options, icon fyne.Resource) {
	nativeApp := app.NewWithID("org.dronecommander.tello-driver")
	nativeApp.Settings().SetTheme(theme.DarkTheme())
	if icon != nil {
		nativeApp.SetIcon(icon)
	}
	languageID := normalizeLanguage(nativeApp.Preferences().StringWithFallback(languagePreference, fyneLang.SystemLocale().LanguageString()))
	ui := &UI{app: nativeApp, window: nativeApp.NewWindow("Drone Commander · Tello Driver"), session: session.New(options), languageID: languageID, stopRefresh: make(chan struct{})}
	ui.build()
	ui.bindKeyboard()
	ui.window.Resize(ui.preferredSize)
	ui.window.CenterOnScreen()
	ui.window.SetCloseIntercept(ui.close)
	ui.refresh(ui.session.Snapshot())
	go ui.refreshLoop()
	ui.window.ShowAndRun()
}

func (u *UI) build() {
	if u.languageID == "" {
		u.languageID = normalizeLanguage(fyneLang.SystemLocale().LanguageString())
	}
	u.lastLogText = "\x00"
	u.connection = widget.NewLabelWithStyle(u.t("not_connected"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.simulation = widget.NewCheck(u.t("simulation"), nil)
	u.connectButton = widget.NewButtonWithIcon(u.t("connect"), theme.RadioButtonCheckedIcon(), u.connect)
	u.connectButton.Importance = widget.HighImportance
	u.disconnectButton = widget.NewButton(u.t("disconnect"), u.disconnect)
	connectionButtons := container.NewHBox(u.connectButton, u.disconnectButton)
	languageNames := make([]string, 0, len(supportedLanguages))
	for _, option := range supportedLanguages {
		languageNames = append(languageNames, option.Name)
	}
	u.language = widget.NewSelect(languageNames, nil)
	u.language.SetSelected(languageName(u.languageID))
	u.language.OnChanged = u.changeLanguage
	languageRow := container.NewBorder(nil, nil, widget.NewLabel(u.t("language")), nil, u.language)
	connectionFooter := container.NewBorder(nil, nil, connectionButtons, languageRow)
	connectionCard := widget.NewCard(u.t("connection"), "", container.NewVBox(u.connection, u.simulation, connectionFooter))

	u.battery = metric("— %")
	u.height = metric("— cm")
	u.yaw = metric("—°")
	u.flightTime = metric("— s")
	telemetry := widget.NewCard(u.t("telemetry"), "", container.NewGridWithColumns(4, metricBox(u.t("battery"), u.battery), metricBox(u.t("altitude"), u.height), metricBox(u.t("heading"), u.yaw), metricBox(u.t("flight_time"), u.flightTime)))
	topRow := container.NewGridWithColumns(2, connectionCard, telemetry)

	u.programName = widget.NewLabelWithStyle(u.t("no_program"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.programName.Truncation = fyne.TextTruncateEllipsis
	u.programMeta = widget.NewLabel(u.t("load_program"))
	u.programMeta.Truncation = fyne.TextTruncateEllipsis
	u.warnings = widget.NewLabel("")
	u.warnings.Truncation = fyne.TextTruncateEllipsis
	u.loadButton = widget.NewButtonWithIcon(u.t("choose_xml"), theme.FolderOpenIcon(), u.chooseProgram)
	u.editButton = widget.NewButtonWithIcon(u.t("view_edit"), theme.DocumentCreateIcon(), u.openProgramEditor)
	u.editButton.Disable()
	programInfo := container.NewGridWithColumns(3, u.programName, u.programMeta, u.warnings)
	programButtons := container.NewHBox(u.loadButton, u.editButton)
	programRow := container.NewBorder(nil, nil, nil, programButtons, programInfo)
	programCard := widget.NewCard(u.t("program"), "", programRow)

	u.minimumBattery = widget.NewEntry()
	u.minimumBattery.SetText("20")
	u.minimumBattery.SetPlaceHolder("20")
	u.minimumBattery.Validator = func(value string) error {
		number, err := strconv.Atoi(value)
		if err != nil || number < 0 || number > 100 {
			return errors.New(u.t("battery_range"))
		}
		return nil
	}
	u.autoLand = widget.NewCheck(u.t("auto_land"), nil)
	u.autoLand.SetChecked(true)
	unitHelp := widget.NewLabelWithStyle(u.t("unit_help"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	unitHelp.Wrapping = fyne.TextWrapWord
	batteryRow := container.NewBorder(nil, nil, widget.NewLabel(u.t("minimum_battery")), nil, u.minimumBattery)
	settings := container.NewVBox(unitHelp, container.NewGridWithColumns(2, batteryRow, u.autoLand))

	u.runButton = widget.NewButtonWithIcon(u.t("start"), theme.MediaPlayIcon(), u.runProgram)
	u.runButton.Importance = widget.HighImportance
	u.stopButton = widget.NewButtonWithIcon(u.t("stop"), theme.MediaStopIcon(), func() { u.safety("stop") })
	u.landButton = widget.NewButtonWithIcon(u.t("land"), theme.MoveDownIcon(), func() { u.safety("land") })
	u.emergencyButton = widget.NewButtonWithIcon(u.t("emergency"), theme.WarningIcon(), u.confirmEmergency)
	u.emergencyButton.Importance = widget.DangerImportance
	controls := container.NewGridWithColumns(4, u.runButton, u.stopButton, u.landButton, u.emergencyButton)
	safetyText := widget.NewLabel(u.t("motor_warning"))
	safetyText.Wrapping = fyne.TextWrapWord
	flightCard := widget.NewCard(u.t("flight"), "", container.NewVBox(settings, controls, safetyText))

	u.lastError = widget.NewLabel("")
	u.lastError.Wrapping = fyne.TextWrapWord
	u.lastError.Hide()
	u.logs = widget.NewRichText(emptyLogSegment(u.languageID))
	u.logs.Wrapping = fyne.TextWrapWord
	u.logScroll = container.NewVScroll(u.logs)
	u.logScroll.SetMinSize(fyne.NewSize(0, 220))
	clearLogButton := widget.NewButtonWithIcon(u.t("clear_log"), theme.ContentClearIcon(), u.confirmClearLogs)
	logHeader := container.NewBorder(nil, nil, widget.NewLabel(u.t("log_help")), clearLogButton)
	logBody := container.NewBorder(logHeader, nil, nil, nil, u.logScroll)
	logCard := widget.NewCard(u.t("flight_log"), "", logBody)
	topContent := container.NewVBox(topRow, programCard, flightCard, u.lastError)
	// The log is the expanding centre object: it reaches the lower edge of the
	// window instead of leaving unused space below a vertically packed VBox.
	frame := container.NewBorder(topContent, nil, nil, nil, logCard)
	u.window.SetContent(frame)
	contentSize := frame.MinSize()
	u.preferredSize = fyne.NewSize(
		max(float32(1100), contentSize.Width+48),
		max(float32(760), contentSize.Height+32),
	)
}

func metric(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
}
func metricBox(title string, value *widget.Label) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{}), value)
}

func (u *UI) t(key string) string { return tr(u.languageID, key) }

func (u *UI) formatXML(data []byte) ([]byte, error) {
	formatted, err := formatDroneXML(data)
	if err == nil {
		return formatted, nil
	}
	labels := xmlErrorLabels[normalizeLanguage(u.languageID)]
	if errors.Is(err, errEmptyXML) {
		return nil, errors.New(labels[1])
	}
	return nil, fmt.Errorf("%s: %w", labels[0], err)
}

func (u *UI) changeLanguage(name string) {
	code := languageCode(name)
	if code == u.languageID {
		return
	}
	u.languageID = code
	u.app.Preferences().SetString(languagePreference, code)
	snapshot := u.session.Snapshot()
	simulation := u.simulation.Checked
	minimumBattery := u.minimumBattery.Text
	autoLand := u.autoLand.Checked
	u.build()
	u.simulation.SetChecked(simulation)
	u.minimumBattery.SetText(minimumBattery)
	u.autoLand.SetChecked(autoLand)
	u.refresh(snapshot)
	current := u.window.Canvas().Size()
	u.window.Resize(fyne.NewSize(max(current.Width, u.preferredSize.Width), max(current.Height, u.preferredSize.Height)))
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
		if loadErr := u.loadProgram(reader); loadErr != nil {
			u.showError(loadErr)
			return
		}
		u.programURI = reader.URI()
		u.refresh(u.session.Snapshot())
	}, u.window)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".xml"}))
	fileDialog.Resize(fyne.NewSize(760, 520))
	fileDialog.Show()
}

func (u *UI) loadProgram(reader fyne.URIReadCloser) error {
	data, err := io.ReadAll(io.LimitReader(reader, 2<<20))
	if err != nil {
		return err
	}
	return u.session.LoadProgram(reader.URI().Name(), data)
}

func (u *UI) readProgramXML() ([]byte, error) {
	if u.programURI == nil {
		return nil, errors.New(u.t("select_xml"))
	}
	return readXML(u.programURI)
}

func readXML(uri fyne.URI) ([]byte, error) {
	reader, err := storage.Reader(uri)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, 2<<20))
}

func (u *UI) saveProgramXML(data []byte) error {
	if u.programURI == nil {
		return errors.New(u.t("select_xml"))
	}
	_, err := u.saveProgramXMLAt(u.programURI, data)
	return err
}

func (u *UI) saveProgramXMLAt(uri fyne.URI, data []byte) (bool, error) {
	formatted, err := u.formatXML(data)
	if err != nil {
		return false, fmt.Errorf(u.t("xml_not_saved"), err)
	}
	if _, err := program.Parse(formatted); err != nil {
		return false, fmt.Errorf(u.t("xml_not_saved"), err)
	}
	writer, err := storage.Writer(uri)
	if err != nil {
		return false, fmt.Errorf(u.t("open_write"), err)
	}
	if _, err = writer.Write(formatted); err != nil {
		_ = writer.Close()
		return false, fmt.Errorf(u.t("write_xml"), err)
	}
	if err := writer.Close(); err != nil {
		return false, fmt.Errorf(u.t("close_xml"), err)
	}
	selected := u.programURI != nil && u.programURI.String() == uri.String()
	if selected && !u.session.Snapshot().Running {
		if err := u.session.LoadProgram(uri.Name(), formatted); err != nil {
			return false, fmt.Errorf(u.t("saved_not_reloaded"), err)
		}
		return true, nil
	}
	return false, nil
}

func (u *UI) openProgramEditor() {
	if u.programURI == nil {
		u.showError(errors.New(u.t("select_xml")))
		return
	}
	editorURI := u.programURI
	data, err := readXML(editorURI)
	if err != nil {
		u.showError(err)
		return
	}
	formatted, err := u.formatXML(data)
	if err != nil {
		u.showError(err)
		return
	}
	labels := commandEditorTranslations[normalizeLanguage(u.languageID)]
	editorWindow := u.app.NewWindow(labels.Title + " · " + editorURI.Name())
	commandEditor := widget.NewMultiLineEntry()
	commandEditor.TextStyle = fyne.TextStyle{Monospace: true}
	commandEditor.Wrapping = fyne.TextWrapOff
	xmlEditor := widget.NewMultiLineEntry()
	xmlEditor.TextStyle = fyne.TextStyle{Monospace: true}
	xmlEditor.Wrapping = fyne.TextWrapOff
	xmlEditor.SetText(string(formatted))
	commandTab := container.NewTabItem(labels.CommandsTab, commandEditor)
	xmlTab := container.NewTabItem(labels.XMLTab, xmlEditor)
	tabs := container.NewAppTabs(commandTab, xmlTab)
	status := widget.NewLabel(labels.Help)
	// A wrapping label in the top Border asks Fyne for a very tall minimum
	// height before the window has a width. That can leave the editor with a
	// negative height and render the source one character per row.
	status.Truncation = fyne.TextTruncateEllipsis
	position := widget.NewLabel("")
	updatePosition := func(editor *widget.Entry) {
		position.SetText(fmt.Sprintf(u.t("line_column"), editor.CursorRow+1, editor.CursorColumn+1))
	}
	commandEditor.OnCursorChanged = func() { updatePosition(commandEditor) }
	xmlEditor.OnCursorChanged = func() { updatePosition(xmlEditor) }
	tabs.OnSelected = func(item *container.TabItem) {
		if item == commandTab {
			updatePosition(commandEditor)
		} else {
			updatePosition(xmlEditor)
		}
	}
	setCommandsFromXML := func(xmlData []byte, selectText bool) error {
		commands, conversionErr := program.ToTextProgram(xmlData)
		if conversionErr != nil {
			message := fmt.Sprintf(labels.Unavailable, conversionErr)
			commandEditor.SetText("# " + message + "\n")
			commandEditor.Disable()
			status.SetText(message)
			tabs.Select(xmlTab)
			return conversionErr
		}
		commandEditor.Enable()
		commandEditor.SetText(commands)
		if selectText {
			tabs.Select(commandTab)
		}
		return nil
	}
	if err := setCommandsFromXML(formatted, true); err != nil {
		tabs.Select(xmlTab)
	} else {
		status.SetText(labels.Help)
	}
	if tabs.Selected() == commandTab {
		updatePosition(commandEditor)
	} else {
		updatePosition(xmlEditor)
	}

	reloadButton := widget.NewButtonWithIcon(u.t("reload"), theme.ViewRefreshIcon(), func() {
		updated, readErr := readXML(editorURI)
		if readErr != nil {
			u.showErrorAt(readErr, editorWindow)
			return
		}
		formatted, formatErr := u.formatXML(updated)
		if formatErr != nil {
			u.showErrorAt(formatErr, editorWindow)
			return
		}
		xmlEditor.SetText(string(formatted))
		if err := setCommandsFromXML(formatted, true); err == nil {
			status.SetText(u.t("reread_formatted"))
		}
	})
	syncButton := widget.NewButtonWithIcon(labels.Sync, theme.ViewRefreshIcon(), func() {
		if tabs.Selected() == commandTab {
			generated, conversionErr := program.TextProgramToXML(commandEditor.Text)
			if conversionErr != nil {
				u.showErrorAt(fmt.Errorf(labels.Invalid, conversionErr), editorWindow)
				return
			}
			formatted, formatErr := u.formatXML(generated)
			if formatErr != nil {
				u.showErrorAt(formatErr, editorWindow)
				return
			}
			xmlEditor.SetText(string(formatted))
			status.SetText(labels.TextToXML)
			return
		}
		formatted, formatErr := u.formatXML([]byte(xmlEditor.Text))
		if formatErr != nil {
			u.showErrorAt(formatErr, editorWindow)
			return
		}
		xmlEditor.SetText(string(formatted))
		if err := setCommandsFromXML(formatted, false); err == nil {
			status.SetText(labels.XMLToText)
		}
	})
	saveButton := widget.NewButtonWithIcon(labels.Save, theme.DocumentSaveIcon(), func() {
		var source []byte
		var conversionErr error
		if tabs.Selected() == commandTab {
			source, conversionErr = program.TextProgramToXML(commandEditor.Text)
			if conversionErr != nil {
				u.showErrorAt(fmt.Errorf(labels.Invalid, conversionErr), editorWindow)
				return
			}
		} else {
			source = []byte(xmlEditor.Text)
		}
		formatted, formatErr := u.formatXML(source)
		if formatErr != nil {
			u.showErrorAt(formatErr, editorWindow)
			return
		}
		reloaded, saveErr := u.saveProgramXMLAt(editorURI, formatted)
		if saveErr != nil {
			u.showErrorAt(saveErr, editorWindow)
			return
		}
		xmlEditor.SetText(string(formatted))
		_ = setCommandsFromXML(formatted, tabs.Selected() == commandTab)
		if reloaded {
			status.SetText(u.t("saved_reloaded"))
			u.refresh(u.session.Snapshot())
		} else if u.session.Snapshot().Running {
			status.SetText(u.t("saved_next"))
		} else {
			status.SetText(u.t("saved_other"))
		}
	})
	saveButton.Importance = widget.HighImportance
	helpButton := widget.NewButtonWithIcon("?", theme.HelpIcon(), func() {
		reference := widget.NewRichText(&widget.TextSegment{
			Style: widget.RichTextStyle{Inline: false, TextStyle: fyne.TextStyle{Monospace: true}},
			Text:  program.TextProgramReference(),
		})
		referenceScroll := container.NewVScroll(reference)
		referenceScroll.SetMinSize(fyne.NewSize(620, 500))
		dialog.ShowCustom(labels.CommandsTab, u.t("close"), referenceScroll, editorWindow)
	})
	closeButton := widget.NewButton(u.t("close"), editorWindow.Close)
	actions := container.NewHBox(reloadButton, syncButton, helpButton, saveButton, closeButton)
	header := container.NewBorder(nil, nil, status, position)
	editorWindow.SetContent(container.NewBorder(header, actions, nil, nil, tabs))
	editorWindow.Resize(fyne.NewSize(1100, 760))
	editorWindow.CenterOnScreen()
	editorWindow.Show()
}

func (u *UI) reloadProgram() error {
	if u.programURI == nil {
		return nil
	}
	reader, err := storage.Reader(u.programURI)
	if err != nil {
		return fmt.Errorf(u.t("reread_xml"), err)
	}
	defer reader.Close()
	if err := u.loadProgram(reader); err != nil {
		return fmt.Errorf(u.t("reread_xml"), err)
	}
	return nil
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
	if err := u.reloadProgram(); err != nil {
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
	confirm, cancel := confirmLabels(u.languageID)
	message := widget.NewLabel(u.t("emergency_question"))
	message.Wrapping = fyne.TextWrapWord
	dialog.ShowCustomConfirm(u.t("emergency_title"), confirm, cancel, message, func(confirmed bool) {
		if confirmed {
			u.safety("emergency")
		}
	}, u.window)
}
func (u *UI) confirmClearLogs() {
	confirm, cancel := confirmLabels(u.languageID)
	message := widget.NewLabel(u.t("clear_question"))
	message.Wrapping = fyne.TextWrapWord
	dialog.ShowCustomConfirm(u.t("clear_title"), confirm, cancel, message, func(confirmed bool) {
		if !confirmed {
			return
		}
		if err := u.session.ClearLogs(); err != nil {
			u.showError(fmt.Errorf(u.t("clear_error"), err))
			return
		}
		u.refresh(u.session.Snapshot())
	}, u.window)
}
func (u *UI) showError(err error) { u.showErrorAt(err, u.window) }
func (u *UI) showErrorAt(err error, parent fyne.Window) {
	message := widget.NewLabel(err.Error())
	message.Wrapping = fyne.TextWrapWord
	dialog.ShowCustom(u.t("error"), u.t("close"), message, parent)
}

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
			u.connection.SetText(u.t("connected_simulation"))
		} else {
			u.connection.SetText(u.t("connected_tello"))
		}
		u.battery.SetText(fmt.Sprintf("%d %%", snapshot.Telemetry.Battery))
		u.height.SetText(fmt.Sprintf("%d cm", snapshot.Telemetry.Height))
		u.yaw.SetText(fmt.Sprintf("%d°", snapshot.Telemetry.Yaw))
		u.flightTime.SetText(fmt.Sprintf("%d s", snapshot.Telemetry.FlightTime))
	} else {
		u.connection.SetText(u.t("not_connected"))
		u.battery.SetText("— %")
		u.height.SetText("— cm")
		u.yaw.SetText("—°")
		u.flightTime.SetText("— s")
	}
	if snapshot.Summary != nil {
		u.programName.SetText(snapshot.ProgramName)
		u.programMeta.SetText(fmt.Sprintf(u.t("program_meta"), snapshot.Summary.Blocks, snapshot.Summary.Commands))
		u.warnings.SetText(translateSummaryWarnings(u.languageID, snapshot.Summary.Warnings))
	} else {
		u.programName.SetText(u.t("no_program"))
		u.programMeta.SetText(u.t("load_program"))
		u.warnings.SetText("")
	}
	if snapshot.LastError == "" {
		u.lastError.Hide()
	} else {
		u.lastError.SetText(snapshot.LastError)
		u.lastError.Show()
	}
	var builder strings.Builder
	for _, entry := range snapshot.Logs {
		fmt.Fprintf(&builder, "[%s] %s\n", entry.Time, entry.Message)
	}
	logText := builder.String()
	if logText != u.lastLogText {
		u.logs.Segments = readableLogSegments(u.languageID, snapshot.Logs)
		u.lastLogText = logText
		u.logs.Refresh()
		u.logScroll.ScrollToBottom()
	}
	setEnabled(u.loadButton, !snapshot.Running && !snapshot.Connecting)
	setEnabled(u.editButton, u.programURI != nil)
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

func emptyLogSegment(language string) widget.RichTextSegment {
	return &widget.TextSegment{
		Style: widget.RichTextStyle{
			ColorName: theme.ColorNameForeground,
			Inline:    false,
			TextStyle: fyne.TextStyle{Italic: true},
		},
		Text: tr(language, "log_empty"),
	}
}

func readableLogSegments(language string, entries []session.LogEntry) []widget.RichTextSegment {
	if len(entries) == 0 {
		return []widget.RichTextSegment{emptyLogSegment(language)}
	}
	segments := make([]widget.RichTextSegment, 0, len(entries))
	for _, entry := range entries {
		category, message, colorName, bold := logPresentation(language, entry.Message)
		segments = append(segments, &widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: colorName,
				Inline:    false,
				TextStyle: fyne.TextStyle{Bold: bold, Monospace: true},
			},
			Text: fmt.Sprintf("%s  %-9s  %s", entry.Time, category, message),
		})
	}
	return segments
}

func logPresentation(language, message string) (category, text string, colorName fyne.ThemeColorName, bold bool) {
	switch {
	case strings.HasPrefix(message, "→ "):
		return tr(language, "command"), strings.TrimPrefix(message, "→ "), theme.ColorNamePrimary, true
	case strings.HasPrefix(message, "← "):
		return tr(language, "response"), strings.TrimPrefix(message, "← "), theme.ColorNameSuccess, false
	case strings.HasPrefix(message, "✕ "):
		return tr(language, "error"), strings.TrimPrefix(message, "✕ "), theme.ColorNameError, true
	case strings.HasPrefix(message, "STEP "):
		if logLooksLikeError(message) {
			return tr(language, "analysis"), message, theme.ColorNameError, true
		}
		if strings.Contains(strings.ToLower(message), "analisi:") {
			return tr(language, "analysis"), message, theme.ColorNameWarning, false
		}
		return tr(language, "analysis"), message, theme.ColorNameSuccess, false
	case logLooksLikeError(message):
		return tr(language, "error"), message, theme.ColorNameError, true
	case strings.Contains(strings.ToLower(message), "batteria"):
		return tr(language, "telemetry_log"), message, theme.ColorNameWarning, false
	default:
		return tr(language, "status"), message, theme.ColorNameForeground, false
	}
}

func logLooksLikeError(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "errore") ||
		strings.Contains(lower, "fallit") ||
		strings.Contains(lower, "non riuscit") ||
		strings.Contains(lower, "non confermat") ||
		strings.Contains(lower, "timeout")
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
