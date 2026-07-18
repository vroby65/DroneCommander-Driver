// Package tello implements the Ryze Tello SDK 2.0 UDP protocol.
package tello

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Telemetry is the latest state packet received on UDP port 8890.
type Telemetry struct {
	Pitch       int                `json:"pitch"`
	Roll        int                `json:"roll"`
	Yaw         int                `json:"yaw"`
	Height      int                `json:"height"`
	Battery     int                `json:"battery"`
	FlightTime  int                `json:"flightTime"`
	Temperature int                `json:"temperature"`
	Values      map[string]float64 `json:"-"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

// Commander is implemented by the real client and by the offline simulator.
type Commander interface {
	Connect(context.Context) error
	Command(context.Context, string) (string, error)
	Immediate(string) error
	Snapshot() Telemetry
	Close() error
}

type Client struct {
	commandAddress string
	stateAddress   string
	timeout        time.Duration

	commandConn  *net.UDPConn
	stateConn    *net.UDPConn
	commandMu    sync.Mutex
	writeMu      sync.Mutex
	stateMu      sync.RWMutex
	state        Telemetry
	pendingDrain atomic.Bool
	closed       chan struct{}
	closeOnce    sync.Once
}

func NewClient(commandAddress, stateAddress string, timeout time.Duration) *Client {
	if commandAddress == "" {
		commandAddress = "192.168.10.1:8889"
	}
	if stateAddress == "" {
		stateAddress = ":8890"
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Client{commandAddress: commandAddress, stateAddress: stateAddress, timeout: timeout, closed: make(chan struct{})}
}

// Connect opens command and state sockets and enters SDK mode.
func (c *Client) Connect(ctx context.Context) error {
	remote, err := net.ResolveUDPAddr("udp", c.commandAddress)
	if err != nil {
		return fmt.Errorf("indirizzo Tello non valido: %w", err)
	}
	commandConn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return fmt.Errorf("connessione UDP al Tello: %w", err)
	}
	stateLocal, err := net.ResolveUDPAddr("udp", c.stateAddress)
	if err != nil {
		commandConn.Close()
		return fmt.Errorf("indirizzo telemetria non valido: %w", err)
	}
	stateConn, err := net.ListenUDP("udp", stateLocal)
	if err != nil {
		commandConn.Close()
		return fmt.Errorf("porta telemetria %s non disponibile: %w", c.stateAddress, err)
	}
	c.commandConn, c.stateConn = commandConn, stateConn
	go c.readState()
	if _, err := c.Command(ctx, "command"); err != nil {
		c.Close()
		return fmt.Errorf("impossibile attivare la modalita SDK (verifica la rete Wi-Fi TELLO): %w", err)
	}
	response, err := c.Command(ctx, "battery?")
	if err == nil {
		if battery, parseErr := strconv.Atoi(strings.TrimSpace(response)); parseErr == nil {
			c.stateMu.Lock()
			c.state.Battery = battery
			c.state.UpdatedAt = time.Now()
			c.stateMu.Unlock()
		}
	}
	return nil
}

// Command sends a serialized SDK command and waits for its response.
func (c *Client) Command(ctx context.Context, command string) (string, error) {
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	if c.commandConn == nil {
		return "", errors.New("Tello non connesso")
	}
	if c.pendingDrain.Swap(false) {
		c.drainResponses(200 * time.Millisecond)
	}

	deadline := time.Now().Add(c.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := c.commandConn.SetDeadline(deadline); err != nil {
		return "", err
	}
	c.writeMu.Lock()
	_, err := c.commandConn.Write([]byte(command))
	c.writeMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("invio %q: %w", command, err)
	}

	buffer := make([]byte, 1024)
	read := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := c.commandConn.Read(buffer)
		read <- struct {
			n   int
			err error
		}{n, err}
	}()
	select {
	case <-ctx.Done():
		_ = c.commandConn.SetReadDeadline(time.Now())
		select {
		case <-read:
		case <-time.After(100 * time.Millisecond):
		}
		c.pendingDrain.Store(true)
		return "", ctx.Err()
	case result := <-read:
		if result.err != nil {
			c.pendingDrain.Store(true)
			return "", fmt.Errorf("risposta a %q: %w", command, result.err)
		}
		response := strings.TrimSpace(string(buffer[:result.n]))
		if strings.HasPrefix(strings.ToLower(response), "error") {
			return response, fmt.Errorf("il Tello ha rifiutato %q: %s", command, response)
		}
		return response, nil
	}
}

// Immediate sends a safety command without waiting behind a running command.
func (c *Client) Immediate(command string) error {
	if c.commandConn == nil {
		return errors.New("Tello non connesso")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.commandConn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err := c.commandConn.Write([]byte(command))
	if err != nil {
		return fmt.Errorf("invio immediato %q: %w", command, err)
	}
	c.pendingDrain.Store(true)
	return nil
}

func (c *Client) drainResponses(quiet time.Duration) {
	buffer := make([]byte, 1024)
	for {
		_ = c.commandConn.SetReadDeadline(time.Now().Add(quiet))
		if _, err := c.commandConn.Read(buffer); err != nil {
			return
		}
	}
}

func (c *Client) Snapshot() Telemetry {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	copy := c.state
	copy.Values = make(map[string]float64, len(c.state.Values))
	for key, value := range c.state.Values {
		copy.Values[key] = value
	}
	return copy
}

func (c *Client) readState() {
	buffer := make([]byte, 2048)
	for {
		_ = c.stateConn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := c.stateConn.ReadFromUDP(buffer)
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				select {
				case <-c.closed:
					return
				default:
					continue
				}
			}
			return
		}
		state := ParseTelemetry(string(buffer[:n]))
		c.stateMu.Lock()
		c.state = state
		c.stateMu.Unlock()
	}
}

// ParseTelemetry decodes the semicolon-separated Tello state format.
func ParseTelemetry(packet string) Telemetry {
	state := Telemetry{Values: make(map[string]float64), UpdatedAt: time.Now()}
	for _, item := range strings.Split(strings.TrimSpace(packet), ";") {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		state.Values[parts[0]] = value
	}
	state.Pitch = int(state.Values["pitch"])
	state.Roll = int(state.Values["roll"])
	state.Yaw = int(state.Values["yaw"])
	state.Height = int(state.Values["h"])
	state.Battery = int(state.Values["bat"])
	state.FlightTime = int(state.Values["time"])
	state.Temperature = int((state.Values["templ"] + state.Values["temph"]) / 2)
	return state
}

func (c *Client) Close() error {
	var first error
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.stateConn != nil {
			if err := c.stateConn.Close(); err != nil {
				first = err
			}
		}
		if c.commandConn != nil {
			if err := c.commandConn.Close(); err != nil && first == nil {
				first = err
			}
		}
	})
	return first
}

// Simulator accepts commands without network access.
type Simulator struct {
	mu     sync.Mutex
	state  Telemetry
	closed bool
}

func NewSimulator() *Simulator {
	return &Simulator{state: Telemetry{Battery: 100, Height: 0, Values: make(map[string]float64), UpdatedAt: time.Now()}}
}
func (s *Simulator) Connect(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = false
	return nil
}
func (s *Simulator) Command(_ context.Context, command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("simulatore disconnesso")
	}
	if command == "battery?" {
		return strconv.Itoa(s.state.Battery), nil
	}
	fields := strings.Fields(command)
	if len(fields) > 0 {
		switch fields[0] {
		case "takeoff":
			s.state.Height = 80
		case "land", "emergency":
			s.state.Height = 0
		case "up":
			if len(fields) > 1 {
				v, _ := strconv.Atoi(fields[1])
				s.state.Height += v
			}
		case "down":
			if len(fields) > 1 {
				v, _ := strconv.Atoi(fields[1])
				s.state.Height = max(0, s.state.Height-v)
			}
		}
	}
	s.state.UpdatedAt = time.Now()
	return "ok", nil
}
func (s *Simulator) Immediate(command string) error {
	_, err := s.Command(context.Background(), command)
	return err
}
func (s *Simulator) Snapshot() Telemetry { s.mu.Lock(); defer s.mu.Unlock(); return s.state }
func (s *Simulator) Close() error        { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }
