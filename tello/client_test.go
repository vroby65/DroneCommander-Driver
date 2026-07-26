package tello

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseTelemetry(t *testing.T) {
	state := ParseTelemetry("pitch:1;roll:-2;yaw:45;templ:60;temph:64;h:83;bat:72;time:14;\r\n")
	if state.Pitch != 1 || state.Roll != -2 || state.Yaw != 45 {
		t.Fatalf("attitude: %+v", state)
	}
	if state.Height != 83 || state.Battery != 72 || state.FlightTime != 14 || state.Temperature != 62 {
		t.Fatalf("telemetry: %+v", state)
	}
}

func TestBatteryQueryUpdatesSnapshot(t *testing.T) {
	client := NewClient("", "", 0)
	client.recordResponse("battery?", "73")
	state := client.Snapshot()
	if state.Battery != 73 || state.UpdatedAt.IsZero() {
		t.Fatalf("snapshot after battery query = %#v", state)
	}
}

func TestMovementTimeoutScalesWithDistanceAndSpeed(t *testing.T) {
	client := NewClient("", "", 8*time.Second)
	if got := client.timeoutFor("battery?"); got != 8*time.Second {
		t.Fatalf("battery timeout = %s, want 8s", got)
	}
	if got := client.timeoutFor("forward 100"); got < 15*time.Second {
		t.Fatalf("100 cm walk timeout = %s, want at least 15s", got)
	}
	client.recordResponse("speed 50", "ok")
	if got := client.timeoutFor("forward 100"); got != 8*time.Second {
		t.Fatalf("100 cm walk at 50 cm/s timeout = %s, want base 8s", got)
	}
	if got := client.timeoutFor("go 500 500 500 10"); got < 90*time.Second {
		t.Fatalf("long go timeout = %s, want at least 90s", got)
	}
}

func TestClientReusesStableLocalCommandPortAfterReconnect(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	localProbe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	localAddress := localProbe.LocalAddr().String()
	if err := localProbe.Close(); err != nil {
		t.Fatal(err)
	}

	const reconnects = 10
	sourcePorts := make(chan int, reconnects*2)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buffer := make([]byte, 1024)
		for {
			n, source, readErr := server.ReadFromUDP(buffer)
			if readErr != nil {
				return
			}
			sourcePorts <- source.Port
			response := "ok"
			if string(buffer[:n]) == "battery?" {
				response = "75"
			}
			_, _ = server.WriteToUDP([]byte(response), source)
		}
	}()

	for attempt := 0; attempt < reconnects; attempt++ {
		client := NewClient(server.LocalAddr().String(), "127.0.0.1:0", time.Second)
		client.commandLocalAddress = localAddress
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := client.Connect(ctx)
		cancel()
		if err != nil {
			t.Fatalf("connect %d: %v", attempt+1, err)
		}
		if err := client.Close(); err != nil {
			t.Fatalf("close %d: %v", attempt+1, err)
		}
	}

	wantPort, err := strconv.Atoi(localAddress[strings.LastIndexByte(localAddress, ':')+1:])
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < reconnects*2; index++ {
		select {
		case port := <-sourcePorts:
			if port != wantPort {
				t.Fatalf("command source port %d = %d, want stable port %d", index+1, port, wantPort)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for fake Tello command")
		}
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	<-serverDone
}

func TestConnectRetriesDroppedSafeCommand(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 8)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buffer := make([]byte, 1024)
		commandAttempts := 0
		for {
			n, source, readErr := server.ReadFromUDP(buffer)
			if readErr != nil {
				return
			}
			command := string(buffer[:n])
			received <- command
			if command == "command" {
				commandAttempts++
				if commandAttempts == 1 {
					continue
				}
			}
			response := "ok"
			if command == "battery?" {
				response = "75"
			}
			_, _ = server.WriteToUDP([]byte(response), source)
		}
	}()

	client := NewClient(server.LocalAddr().String(), "127.0.0.1:0", 100*time.Millisecond)
	client.commandLocalAddress = "127.0.0.1:0"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err = client.Connect(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	<-serverDone
	close(received)

	var commands []string
	for command := range received {
		commands = append(commands, command)
	}
	if got := strings.Join(commands, "|"); got != "command|command|battery?" {
		t.Fatalf("commands after dropped response = %q", got)
	}
}

func TestOnlyIdempotentCommandsAreRetried(t *testing.T) {
	for _, test := range []struct {
		command string
		want    bool
	}{
		{command: "command", want: true},
		{command: "streamon", want: true},
		{command: "streamoff", want: true},
		{command: "battery?", want: true},
		{command: "forward 20", want: false},
		{command: "takeoff", want: false},
		{command: "land", want: false},
	} {
		if got := retryableCommand(test.command); got != test.want {
			t.Errorf("retryableCommand(%q) = %t, want %t", test.command, got, test.want)
		}
	}
}

func TestCommandResponsesMustMatchTheirCommandType(t *testing.T) {
	for _, test := range []struct {
		command  string
		response string
		want     bool
	}{
		{command: "streamon", response: "ok", want: true},
		{command: "forward 20", response: "75", want: false},
		{command: "battery?", response: "75", want: true},
		{command: "battery?", response: "ok", want: false},
	} {
		if got := responseMatchesCommand(test.command, test.response); got != test.want {
			t.Errorf("responseMatchesCommand(%q, %q) = %t, want %t", test.command, test.response, got, test.want)
		}
	}
}

func TestMatchingLocalIPSelectsTelloSubnet(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.178.91"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.10.3"), Mask: net.CIDRMask(24, 32)},
	}
	got := matchingLocalIP(net.ParseIP("192.168.10.1"), addresses)
	if !got.Equal(net.ParseIP("192.168.10.3")) {
		t.Fatalf("matching local IP = %v, want 192.168.10.3", got)
	}
}

func TestSimulatorTracksAltitudeForVectorAndCurveCommands(t *testing.T) {
	simulator := NewSimulator()
	if err := simulator.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"takeoff",
		"go 30 0 40 10",
		"curve 0 20 10 0 40 -20 10",
	} {
		if _, err := simulator.Command(context.Background(), command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if got := simulator.Snapshot().Height; got != 100 {
		t.Fatalf("simulated altitude = %d cm, want 100 cm", got)
	}
}

func TestSimulatorTracksClockwiseAndCounterclockwiseYaw(t *testing.T) {
	simulator := NewSimulator()
	if err := simulator.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"cw 90", "ccw 30"} {
		if _, err := simulator.Command(context.Background(), command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if got := simulator.Snapshot().Yaw; got != 60 {
		t.Fatalf("simulated yaw = %d degrees, want 60", got)
	}
}
