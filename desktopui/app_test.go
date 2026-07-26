package desktopui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	fyneTest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/session"
)

func TestPreferredWindowContainsInterface(t *testing.T) {
	application := fyneTest.NewApp()
	defer application.Quit()
	window := application.NewWindow("test")
	ui := &UI{app: application, window: window, session: session.New(session.Options{})}
	ui.build()
	if ui.preferredSize.Width < 1100 || ui.preferredSize.Height < 760 {
		t.Fatalf("preferred window = %.0fx%.0f, below 1100x760", ui.preferredSize.Width, ui.preferredSize.Height)
	}
	if ui.cameraToggle == nil || ui.cameraImage == nil || ui.cameraStatus == nil {
		t.Fatal("lower-right camera panel was not built")
	}
	if ui.mediaDirectory == nil || ui.mediaButton == nil {
		t.Fatal("media directory selector was not built")
	}
	if ui.cameraImage.Size().Width < 320 || ui.cameraImage.Size().Height < 240 {
		t.Fatalf("camera viewport = %.0fx%.0f, want at least 320x240", ui.cameraImage.Size().Width, ui.cameraImage.Size().Height)
	}
	ui.refresh(ui.session.Snapshot())
	if !ui.cameraToggle.Disabled() {
		t.Fatal("camera toggle must stay disabled until a real Tello is connected")
	}
}

func TestRealMediaProgramRequiresDirectoryChoiceBeforeRun(t *testing.T) {
	mediaSummary := &program.Summary{MediaCommands: 1}
	if !shouldChooseMediaDirectory(session.Snapshot{Summary: mediaSummary}) {
		t.Fatal("real program with media commands did not require a destination")
	}
	if shouldChooseMediaDirectory(session.Snapshot{Summary: mediaSummary, Simulated: true}) {
		t.Fatal("simulation should not ask for a real camera media destination")
	}
	if shouldChooseMediaDirectory(session.Snapshot{Summary: &program.Summary{}}) {
		t.Fatal("program without media commands requested a destination")
	}
}

func TestReadableLogSegmentsShowEventCategories(t *testing.T) {
	entries := []session.LogEntry{
		{Time: "12:01:02", Message: "→ forward 30"},
		{Time: "12:01:03", Message: "← ok"},
		{Time: "12:01:04", Message: "STEP walk: analisi OK"},
		{Time: "12:01:05", Message: "Errore di esecuzione: timeout"},
	}
	segments := readableLogSegments("it", entries)
	if len(segments) != len(entries) {
		t.Fatalf("segments = %d, want %d", len(segments), len(entries))
	}
	var rendered strings.Builder
	for _, segment := range segments {
		rendered.WriteString(segment.Textual())
		rendered.WriteByte('\n')
	}
	text := rendered.String()
	for _, category := range []string{"COMANDO", "RISPOSTA", "ANALISI", "ERRORE"} {
		if !strings.Contains(text, category) {
			t.Fatalf("rendered log does not contain %q:\n%s", category, text)
		}
	}
}

func TestLanguagesMatchDroneCommander(t *testing.T) {
	want := []string{"en", "it", "fr", "de", "es", "pt", "ar", "zh", "ko", "ja"}
	if len(supportedLanguages) != len(want) {
		t.Fatalf("supported languages = %d, want %d", len(supportedLanguages), len(want))
	}
	for index, code := range want {
		if got := supportedLanguages[index].Code; got != code {
			t.Fatalf("language %d = %q, want %q", index, got, code)
		}
	}
}

func TestEveryLanguageContainsAllInterfaceMessages(t *testing.T) {
	for _, option := range supportedLanguages {
		messages := uiMessages[option.Code]
		for key := range uiMessages["en"] {
			if messages[key] == "" {
				t.Errorf("language %s is missing %q", option.Code, key)
			}
		}
	}
}

func TestLanguageCanChangeWithoutLosingFlightSettings(t *testing.T) {
	application := fyneTest.NewApp()
	defer application.Quit()
	window := application.NewWindow("test")
	ui := &UI{app: application, window: window, session: session.New(session.Options{}), languageID: "en"}
	ui.build()
	ui.simulation.SetChecked(true)
	ui.minimumBattery.SetText("35")
	ui.autoLand.SetChecked(false)
	ui.collisionCheck.SetChecked(true)

	ui.changeLanguage("Deutsch")
	if ui.languageID != "de" || ui.connection.Text != "● Nicht verbunden" {
		t.Fatalf("language was not applied: id=%q connection=%q", ui.languageID, ui.connection.Text)
	}
	if !ui.simulation.Checked || ui.minimumBattery.Text != "35" || ui.autoLand.Checked || !ui.collisionCheck.Checked {
		t.Fatal("changing language lost flight settings")
	}
	if got := application.Preferences().String(languagePreference); got != "de" {
		t.Fatalf("saved language = %q, want de", got)
	}
}

func TestItalianFlightOptionLabels(t *testing.T) {
	application := fyneTest.NewApp()
	defer application.Quit()
	window := application.NewWindow("test")
	ui := &UI{app: application, window: window, session: session.New(session.Options{}), languageID: "it"}
	ui.build()

	if got := ui.autoLand.Text; got != "Atterra al termine" {
		t.Fatalf("automatic landing label = %q", got)
	}
	if got := ui.collisionCheck.Text; got != "Controllo collisioni" {
		t.Fatalf("collision check label = %q", got)
	}
}

func TestProgramEditorTextAreaUsesWindowWidth(t *testing.T) {
	application := fyneTest.NewApp()
	defer application.Quit()
	window := application.NewWindow("main")
	window.SetContent(widget.NewLabel("main"))
	window.Show()
	if got := len(application.Driver().AllWindows()); got < 2 {
		t.Fatalf("test driver windows after creating main = %d", got)
	}
	path, err := filepath.Abs("../examples/quadrato.xml")
	if err != nil {
		t.Fatal(err)
	}
	ui := &UI{
		app: application, window: window, session: session.New(session.Options{}),
		programURI: storage.NewFileURI(path), languageID: "it",
	}
	if _, err := readXML(ui.programURI); err != nil {
		t.Fatalf("read editor XML: %v", err)
	}
	before := len(application.Driver().AllWindows())
	ui.openProgramEditor()
	after := len(application.Driver().AllWindows())
	if after < before {
		t.Fatalf("opening editor removed windows: before=%d after=%d", before, after)
	}

	var tabs *container.AppTabs
	var windows []string
	var editorContent fyne.CanvasObject
	for _, candidate := range application.Driver().AllWindows() {
		windows = append(windows, candidate.Title()+":"+fmt.Sprintf("%T", candidate.Content()))
		if candidate.Title() != "main" {
			if found := findAppTabs(candidate.Content()); found != nil {
				tabs = found
				editorContent = candidate.Content()
				break
			}
		}
	}
	if tabs == nil {
		t.Fatalf("program editor tabs not found; windows: %s", strings.Join(windows, ", "))
	}
	entry, ok := tabs.Items[0].Content.(*widget.Entry)
	if !ok {
		t.Fatalf("text command tab content = %T, want *widget.Entry", tabs.Items[0].Content)
	}
	if width := entry.Size().Width; width < 800 {
		t.Fatalf("text command editor width = %.0f, want at least 800", width)
	}
	if height := entry.Size().Height; height < 500 {
		var children []string
		if root, ok := editorContent.(*fyne.Container); ok {
			for _, child := range root.Objects {
				children = append(children, fmt.Sprintf("%T size=%.0fx%.0f min=%.0fx%.0f", child, child.Size().Width, child.Size().Height, child.MinSize().Width, child.MinSize().Height))
			}
		}
		t.Fatalf("text command editor height = %.0f, want at least 500 (tabs %.0fx%.0f, min %.0fx%.0f, entry min %.0fx%.0f; root: %s)", height, tabs.Size().Width, tabs.Size().Height, tabs.MinSize().Width, tabs.MinSize().Height, entry.MinSize().Width, entry.MinSize().Height, strings.Join(children, "; "))
	}
}

func findAppTabs(object fyne.CanvasObject) *container.AppTabs {
	if tabs, ok := object.(*container.AppTabs); ok {
		return tabs
	}
	if content, ok := object.(*fyne.Container); ok {
		for _, child := range content.Objects {
			if tabs := findAppTabs(child); tabs != nil {
				return tabs
			}
		}
	}
	return nil
}

func TestProgramWarningsAreLocalized(t *testing.T) {
	warnings := []string{
		"Il programma non contiene comandi per il drone.",
		"Il programma non contiene Atterra; lascia attivo l'atterraggio automatico.",
	}
	if got := translateSummaryWarnings("en", warnings); got != "The program contains no drone commands. The program has no Land block; keep automatic landing enabled." {
		t.Fatalf("English warnings = %q", got)
	}
}

func TestReloadProgramReadsChangesFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programma.xml")
	writeXML := func(actions string) {
		t.Helper()
		xml := `<xml><block type="start_block"><next>` + actions + `</next></block></xml>`
		if err := os.WriteFile(path, []byte(xml), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ui := &UI{session: session.New(session.Options{}), programURI: storage.NewFileURI(path)}
	writeXML(`<block type="take_off"></block>`)
	if err := ui.reloadProgram(); err != nil {
		t.Fatal(err)
	}
	if got := ui.session.Snapshot().Summary.Commands; got != 1 {
		t.Fatalf("initial commands = %d, want 1", got)
	}

	writeXML(`<block type="take_off"><next><block type="land"></block></next></block>`)
	if err := ui.reloadProgram(); err != nil {
		t.Fatal(err)
	}
	if got := ui.session.Snapshot().Summary.Commands; got != 2 {
		t.Fatalf("commands after file edit = %d, want 2", got)
	}
}

func TestSaveProgramXMLValidatesBeforeOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programma.xml")
	original := `<xml><block type="start_block"></block></xml>`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	ui := &UI{session: session.New(session.Options{}), programURI: storage.NewFileURI(path)}
	updated := `<xml><block type="start_block"><next><block type="take_off"></block></next></block></xml>`
	if err := ui.saveProgramXML([]byte(updated)); err != nil {
		t.Fatal(err)
	}
	if got := ui.session.Snapshot().Summary.Commands; got != 1 {
		t.Fatalf("commands after editor save = %d, want 1", got)
	}
	if err := ui.saveProgramXML([]byte(`<xml><block`)); err == nil {
		t.Fatal("invalid XML was accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := formatDroneXML([]byte(updated))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(formatted) {
		t.Fatalf("saved XML was not formatted as expected:\n%s", data)
	}
}

func TestFormatDroneXMLMakesCompactBlocklyReadable(t *testing.T) {
	compact := `<xml xmlns="https://developers.google.com/blockly/xml"><block type="start_block"><next><block type="text_print"><value name="TEXT"><block type="text"><field name="TEXT">  testo con spazi  </field></block></value></block></next></block></xml>`
	formatted, err := formatDroneXML([]byte(compact))
	if err != nil {
		t.Fatal(err)
	}
	text := string(formatted)
	for _, expected := range []string{
		`<xml xmlns="https://developers.google.com/blockly/xml">`,
		"\n  <block type=\"start_block\">",
		"\n      <block type=\"text_print\">",
		`<field name="TEXT">  testo con spazi  </field>`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("formatted XML does not contain %q:\n%s", expected, text)
		}
	}
	if count := strings.Count(text, `xmlns="https://developers.google.com/blockly/xml"`); count != 1 {
		t.Fatalf("namespace declaration count = %d, want 1:\n%s", count, text)
	}
	if _, err := program.Parse(formatted); err != nil {
		t.Fatalf("formatted XML no longer parses: %v\n%s", err, text)
	}
}
