// Package tello implements the Ryze Tello SDK 2.0 UDP protocol.
package tello

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	commandAddress      string
	commandLocalAddress string
	stateAddress        string
	timeout             time.Duration

	commandConn  *net.UDPConn
	stateConn    *net.UDPConn
	commandMu    sync.Mutex
	writeMu      sync.Mutex
	stateMu      sync.RWMutex
	state        Telemetry
	speedCM      int
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
	return &Client{
		commandAddress: commandAddress, commandLocalAddress: ":9000",
		stateAddress: stateAddress, timeout: timeout, speedCM: 10, closed: make(chan struct{}),
	}
}

// Connect opens command and state sockets and enters SDK mode.
func (c *Client) Connect(ctx context.Context) error {
	remote, err := net.ResolveUDPAddr("udp", c.commandAddress)
	if err != nil {
		return fmt.Errorf("indirizzo Tello non valido: %w", err)
	}
	commandLocal, err := net.ResolveUDPAddr("udp", c.commandLocalAddress)
	if err != nil {
		return fmt.Errorf("indirizzo UDP locale comandi non valido: %w", err)
	}
	if commandLocal.IP == nil || commandLocal.IP.IsUnspecified() {
		commandLocal.IP = localIPOnRemoteSubnet(remote.IP)
	}
	commandConn, err := net.DialUDP("udp", commandLocal, remote)
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
	_, _ = c.Command(ctx, "battery?")
	go c.keepAlive()
	return nil
}

// localIPOnRemoteSubnet keeps command traffic on the Tello Wi-Fi when the
// computer is connected to another network at the same time. Without an
// explicit source address, a transient route change can send 192.168.10.1
// through the Internet-facing adapter.
func localIPOnRemoteSubnet(remote net.IP) net.IP {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return matchingLocalIP(remote, addresses)
}

func matchingLocalIP(remote net.IP, addresses []net.Addr) net.IP {
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || !network.Contains(remote) {
			continue
		}
		if remote.To4() != nil && network.IP.To4() == nil {
			continue
		}
		return append(net.IP(nil), network.IP...)
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
	if err := ctx.Err(); err != nil {
		return "", err
	}

	commandTimeout := c.timeoutFor(command)
	attempts := 1
	if retryableCommand(command) {
		attempts = 3
		commandTimeout = min(commandTimeout, 1500*time.Millisecond)
	}
	var lastResponse string
	var lastError error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return lastResponse, err
		}
		if c.pendingDrain.Swap(false) {
			c.drainResponses(100 * time.Millisecond)
		}
		response, err, retry := c.commandOnce(ctx, command, commandTimeout)
		if err == nil {
			if attempt > 1 {
				// Multiple idempotent sends can produce duplicate late
				// acknowledgements. Drain them before a different command uses
				// the same response socket.
				c.pendingDrain.Store(true)
			}
			return response, nil
		}
		lastResponse, lastError = response, err
		if !retry || attempt == attempts {
			break
		}
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastResponse, ctx.Err()
		case <-timer.C:
		}
	}
	if attempts > 1 {
		return lastResponse, fmt.Errorf("%q non confermato dopo %d tentativi: %w", command, attempts, lastError)
	}
	return lastResponse, lastError
}

func (c *Client) commandOnce(ctx context.Context, command string, commandTimeout time.Duration) (string, error, bool) {
	started := time.Now()
	deadline := started.Add(commandTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := c.commandConn.SetDeadline(deadline); err != nil {
		return "", err, true
	}
	c.writeMu.Lock()
	_, err := c.commandConn.Write([]byte(command))
	c.writeMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("invio %q: %w", command, err), true
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
		return "", ctx.Err(), false
	case result := <-read:
		if result.err != nil {
			c.pendingDrain.Store(true)
			if timeout, ok := result.err.(net.Error); ok && timeout.Timeout() {
				if retryableCommand(command) {
					return "", fmt.Errorf("timeout Tello dopo %s per %q: %w", time.Since(started).Round(100*time.Millisecond), command, result.err), true
				}
				return "", fmt.Errorf(
					"timeout Tello dopo %s per %q (il movimento potrebbe essere ancora in corso): %w",
					time.Since(started).Round(100*time.Millisecond), command, result.err,
				), true
			}
			return "", fmt.Errorf("risposta a %q: %w", command, result.err), true
		}
		response := strings.TrimSpace(string(buffer[:result.n]))
		if strings.HasPrefix(strings.ToLower(response), "error") {
			return response, fmt.Errorf("il Tello ha rifiutato %q: %s", command, response), false
		}
		if !responseMatchesCommand(command, response) {
			c.pendingDrain.Store(true)
			return response, fmt.Errorf("risposta Tello inattesa per %q: %q", command, response), retryableCommand(command)
		}
		c.recordResponse(command, response)
		return response, nil, false
	}
}

func retryableCommand(command string) bool {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "command", "streamon", "streamoff":
		return true
	default:
		return strings.HasSuffix(fields[0], "?")
	}
}

func responseMatchesCommand(command, response string) bool {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 || response == "" {
		return false
	}
	if strings.HasSuffix(fields[0], "?") {
		return !strings.EqualFold(response, "ok")
	}
	return strings.EqualFold(response, "ok")
}

func (c *Client) keepAlive() {
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = c.Command(ctx, "battery?")
			cancel()
		}
	}
}

// timeoutFor allows movement commands enough time to finish before expecting
// their acknowledgement. The base timeout remains appropriate for short SDK
// queries, but a 500 cm walk at 10 cm/s legitimately takes about 50 seconds.
func (c *Client) timeoutFor(command string) time.Duration {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return c.timeout
	}
	withMargin := func(distance float64, speed int, factor float64) time.Duration {
		if speed <= 0 {
			speed = 10
		}
		seconds := distance / float64(speed) * factor
		return max(c.timeout, time.Duration(math.Ceil(seconds))*time.Second+5*time.Second)
	}
	coordinate := func(index int) float64 {
		if index >= len(fields) {
			return 0
		}
		value, _ := strconv.ParseFloat(fields[index], 64)
		return value
	}
	magnitude := func(x, y, z float64) float64 { return math.Sqrt(x*x + y*y + z*z) }

	switch fields[0] {
	case "takeoff", "land":
		return max(c.timeout, 15*time.Second)
	case "forward", "back", "left", "right", "up", "down":
		return withMargin(math.Abs(coordinate(1)), c.speedCM, 1)
	case "go":
		speed := int(coordinate(4))
		return withMargin(magnitude(coordinate(1), coordinate(2), coordinate(3)), speed, 1)
	case "curve":
		x1, y1, z1 := coordinate(1), coordinate(2), coordinate(3)
		x2, y2, z2 := coordinate(4), coordinate(5), coordinate(6)
		pathEstimate := magnitude(x1, y1, z1) + magnitude(x2-x1, y2-y1, z2-z1)
		return withMargin(pathEstimate, int(coordinate(7)), 1.5)
	default:
		return c.timeout
	}
}

// recordResponse keeps explicitly queried values in the same snapshot used by
// the UI and remembers the speed needed to estimate later movement durations.
func (c *Client) recordResponse(command, response string) {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return
	}
	if fields[0] == "speed" && len(fields) > 1 {
		if speed, err := strconv.Atoi(fields[1]); err == nil && speed > 0 {
			c.speedCM = speed
		}
		return
	}
	if fields[0] != "battery?" {
		return
	}
	battery, err := strconv.Atoi(strings.TrimSpace(response))
	if err != nil {
		return
	}
	c.stateMu.Lock()
	c.state.Battery = battery
	c.state.UpdatedAt = time.Now()
	c.stateMu.Unlock()
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
		s.state.UpdatedAt = time.Now()
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
		case "go":
			if len(fields) > 3 {
				up, _ := strconv.Atoi(fields[3])
				s.state.Height = max(0, s.state.Height+up)
			}
		case "curve":
			if len(fields) > 6 {
				finalUp, _ := strconv.Atoi(fields[6])
				s.state.Height = max(0, s.state.Height+finalUp)
			}
		case "cw":
			if len(fields) > 1 {
				degrees, _ := strconv.Atoi(fields[1])
				s.state.Yaw = signedYaw(s.state.Yaw + degrees)
			}
		case "ccw":
			if len(fields) > 1 {
				degrees, _ := strconv.Atoi(fields[1])
				s.state.Yaw = signedYaw(s.state.Yaw - degrees)
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

func signedYaw(degrees int) int {
	return ((degrees+180)%360+360)%360 - 180
}
