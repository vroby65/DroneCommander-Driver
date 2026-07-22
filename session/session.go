// Package session coordinates one loaded program and one Tello connection.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vroby65/DroneCommander-Driver/flight"
	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/tello"
)

type Options struct {
	CommandAddress string
	StateAddress   string
	CommandTimeout time.Duration
	LogPath        string
}

type LogEntry struct {
	Time    string
	Message string
}

type Snapshot struct {
	Connected   bool
	Connecting  bool
	Simulated   bool
	Running     bool
	ProgramName string
	Summary     *program.Summary
	Telemetry   tello.Telemetry
	LastError   string
	Logs        []LogEntry
}

type RunConfig struct {
	MinimumBattery int
	AutoLand       bool
}

type Session struct {
	options     Options
	mu          sync.Mutex
	program     *program.Program
	programName string
	device      tello.Commander
	connected   bool
	simulated   bool
	connecting  bool
	running     bool
	runID       uint64
	cancel      context.CancelFunc
	lastError   string
	logs        []LogEntry
	logPath     string
	keys        map[string]bool
}

func New(options Options) *Session {
	if options.CommandAddress == "" {
		options.CommandAddress = "192.168.10.1:8889"
	}
	if options.StateAddress == "" {
		options.StateAddress = ":8890"
	}
	if options.CommandTimeout <= 0 {
		options.CommandTimeout = 8 * time.Second
	}
	return &Session{options: options, logPath: options.LogPath, keys: make(map[string]bool)}
}

func (s *Session) LoadProgram(name string, data []byte) error {
	parsed, err := program.Parse(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return errors.New("ferma il programma prima di sostituirlo")
	}
	s.program = parsed
	s.programName = name
	s.lastError = ""
	s.addLogLocked(fmt.Sprintf("Caricato %s: %d blocchi, %d comandi.", name, parsed.Summary.Blocks, parsed.Summary.Commands))
	return nil
}

func (s *Session) Connect(ctx context.Context, simulation bool) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("ferma il programma prima di cambiare connessione")
	}
	if s.connecting {
		s.mu.Unlock()
		return errors.New("connessione gia in corso")
	}
	s.connecting = true
	old := s.device
	s.device = nil
	s.connected = false
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	var device tello.Commander
	if simulation {
		device = tello.NewSimulator()
	} else {
		device = tello.NewClient(s.options.CommandAddress, s.options.StateAddress, s.options.CommandTimeout)
	}
	err := device.Connect(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connecting = false
	if err != nil {
		_ = device.Close()
		s.lastError = err.Error()
		s.addLogLocked("Connessione fallita: " + err.Error())
		return err
	}
	s.device = device
	s.connected = true
	s.simulated = simulation
	s.lastError = ""
	if simulation {
		s.addLogLocked("Modalita simulazione: nessun comando viene inviato in rete.")
	} else {
		s.addLogLocked("Tello connesso in modalita SDK.")
	}
	return nil
}

func (s *Session) Disconnect() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.runID++
	device := s.device
	s.device = nil
	s.connected = false
	s.connecting = false
	s.running = false
	s.cancel = nil
	s.addLogLocked("Disconnesso.")
	s.mu.Unlock()
	if device != nil {
		return device.Close()
	}
	return nil
}

func (s *Session) Start(config RunConfig) error {
	if config.MinimumBattery < 0 || config.MinimumBattery > 100 {
		return errors.New("la soglia batteria deve essere tra 0 e 100")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected || s.device == nil {
		return errors.New("connetti il Tello o attiva la simulazione")
	}
	if s.program == nil {
		return errors.New("carica prima un programma XML")
	}
	if s.running {
		return errors.New("un programma e gia in esecuzione")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.lastError = ""
	s.runID++
	runID := s.runID
	device := s.device
	parsed := s.program
	s.addLogLocked("Avvio programma " + s.programName + " (1 unita = 1 cm).")
	controller := flight.NewController(device, flight.Config{MinimumBattery: config.MinimumBattery, KeyPressed: s.KeyPressed, Log: s.addLog})
	go s.execute(ctx, parsed, controller, device, config.AutoLand, runID)
	return nil
}

func (s *Session) execute(ctx context.Context, parsed *program.Program, controller *flight.Controller, device tello.Commander, autoLand bool, runID uint64) {
	interpreter := program.Interpreter{Program: parsed, Host: controller, MaxSteps: 10000}
	err := interpreter.Execute(ctx)
	result := controller.Result()
	if err == nil {
		s.addLog("Programma completato.")
		if autoLand && result.Flying {
			s.addLog("Atterraggio automatico finale.")
			if landErr := s.confirmedLand(device, false); landErr != nil {
				err = fmt.Errorf("atterraggio automatico: %w", landErr)
			}
		}
	} else if errors.Is(err, context.Canceled) {
		s.addLog("Programma annullato da un comando manuale.")
	} else {
		s.addLog("Errore di esecuzione: " + err.Error())
		_ = device.Immediate("stop")
		if result.Flying {
			s.addLog("Atterraggio di sicurezza dopo l'errore.")
			time.Sleep(250 * time.Millisecond)
			if landErr := s.confirmedLand(device, true); landErr != nil {
				s.addLog("Atterraggio di sicurezza non confermato: " + landErr.Error())
				err = fmt.Errorf("%w; atterraggio di sicurezza: %v", err, landErr)
			}
		}
	}

	// Refresh the displayed charge after every program run. Use a fresh context:
	// the program context may have been cancelled by STOP or a manual landing,
	// while the Tello connection is still available for this final read.
	s.mu.Lock()
	refreshBattery := s.runID == runID && s.connected
	s.mu.Unlock()
	if refreshBattery {
		batteryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		battery, batteryErr := controller.Battery(batteryCtx)
		cancel()
		if batteryErr != nil {
			s.addLog("Lettura batteria dopo il volo non riuscita: " + batteryErr.Error())
		} else {
			s.addLog(fmt.Sprintf("Batteria dopo il volo: %d%%", battery))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runID != runID {
		return
	}
	s.running = false
	s.cancel = nil
	if err != nil && !errors.Is(err, context.Canceled) {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
	}
}

func (s *Session) Safety(command string) error {
	if command != "stop" && command != "land" && command != "emergency" {
		return errors.New("comando di sicurezza non valido")
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	device := s.device
	connected := s.connected
	s.addLogLocked(map[string]string{"stop": "STOP: richiesta hovering.", "land": "Atterraggio manuale richiesto.", "emergency": "EMERGENZA: arresto immediato dei motori."}[command])
	s.mu.Unlock()
	if !connected || device == nil {
		return errors.New("nessun Tello connesso")
	}
	if command == "land" {
		return s.confirmedLand(device, true)
	}
	return device.Immediate(command)
}

// confirmedLand sends land immediately when requested, then sends it through
// the serialized command path and waits for the drone's acknowledgement. The
// second send also acts as a retry if the first UDP datagram was lost.
func (s *Session) confirmedLand(device tello.Commander, immediate bool) error {
	if immediate {
		if err := device.Immediate("land"); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := device.Command(ctx, "land")
	if err != nil {
		return err
	}
	s.addLog("Atterraggio confermato dal Tello: " + response)
	return nil
}

func (s *Session) SetKey(key string, pressed bool) {
	s.mu.Lock()
	s.keys[normalizeKey(key)] = pressed
	s.mu.Unlock()
}
func (s *Session) KeyPressed(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys[normalizeKey(key)]
}
func normalizeKey(key string) string {
	key = strings.ToUpper(strings.TrimSpace(key))
	switch key {
	case "UP", "ARROWUP":
		return "ARROW_UP"
	case "DOWN", "ARROWDOWN":
		return "ARROW_DOWN"
	case "LEFT", "ARROWLEFT":
		return "ARROW_LEFT"
	case "RIGHT", "ARROWRIGHT":
		return "ARROW_RIGHT"
	case "SPACE":
		return "SPACE"
	case "ENTER":
		return "RETURN"
	}
	return key
}

func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	snapshot := Snapshot{Connected: s.connected, Connecting: s.connecting, Simulated: s.simulated, Running: s.running, ProgramName: s.programName, LastError: s.lastError, Logs: append([]LogEntry(nil), s.logs...)}
	if s.program != nil {
		summary := s.program.Summary
		snapshot.Summary = &summary
	}
	device := s.device
	s.mu.Unlock()
	if device != nil {
		snapshot.Telemetry = device.Snapshot()
	}
	return snapshot
}

// ClearLogs clears both the in-memory flight history and the persistent log.
// The session lock keeps this operation ordered with log messages produced by
// a flight that may still be running.
func (s *Session) ClearLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logPath == "" {
		s.logs = nil
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.logPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	s.logs = nil
	return nil
}

func (s *Session) addLog(message string) { s.mu.Lock(); defer s.mu.Unlock(); s.addLogLocked(message) }
func (s *Session) addLogLocked(message string) {
	now := time.Now()
	entry := LogEntry{Time: now.Format("15:04:05"), Message: message}
	s.logs = append(s.logs, entry)
	if len(s.logs) > 250 {
		s.logs = append([]LogEntry(nil), s.logs[len(s.logs)-250:]...)
	}
	if s.logPath != "" {
		_ = appendFlightLog(s.logPath, now, message)
	}
}

func appendFlightLog(path string, timestamp time.Time, message string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "%s %s\n", timestamp.Format(time.RFC3339), message)
	return err
}
func (s *Session) Close() error { return s.Disconnect() }
