# Drone Commander Tello Driver

A native Go desktop application that loads XML programs exported by [Drone Commander](https://github.com/vroby65/DroneCommander) and runs them on a Ryze/DJI Tello through Tello SDK 2.0. The interface is built with Fyne, opens as a regular desktop window, and does not start a web server or browser.

Drone Commander provides an [online editor and simulator](https://vroby65.github.io/DroneCommander/). Create and test a program in the browser, save it as an XML file, and open it in this driver for offline simulation or real flight.

## Drone Commander Workflow

1. Create the program in the [online editor](https://vroby65.github.io/DroneCommander/).
2. Test it in the 3D simulator and select **Save** to export an XML file.
3. Open the file in the driver and test it again in simulation mode.
4. Connect the computer to the `TELLO-...` Wi-Fi network and run the program on the drone.

## Indoor Units

The conversion is fixed:

```text
1 Drone Commander unit = 1 centimeter
```

The Tello accepts the linear commands `up`, `down`, `left`, `right`, `forward`, and `back` only for distances from 20 to 500 cm. A `Walk 1` block therefore stops with an error; use `Walk 30` to move forward by 30 cm.

The `Return to base` block returns to the initial X/Z coordinates while maintaining the current altitude. This avoids reproducing the simulator's legacy altitude of 10 units, which would mean only 10 cm indoors.

The driver also rejects programmed flight altitudes below 20 cm. Use the `Land` block to descend below that threshold.

## Run

Building requires Go 1.22, a C compiler, and the graphics libraries required by Fyne.

On Debian/Ubuntu:

```sh
sudo apt-get install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev
git clone https://github.com/vroby65/DroneCommander-Driver.git
cd DroneCommander-Driver
go run .
```

In the application:

1. Choose the interface language. The driver supports the same languages as Drone Commander: English, Italiano, Français, Deutsch, Español, Português, العربية, 简体中文, 한국어, and 日本語. The system language is selected initially and the choice is remembered.
2. Choose an XML file saved by Drone Commander.
3. Enable simulation mode, connect, and test the program.
4. For real flight, connect the computer to the `TELLO-...` Wi-Fi network, disable simulation mode, and connect to the drone.
5. Check the minimum battery threshold and start the program.

The [examples/quadrato.xml](examples/quadrato.xml) file contains a 50 cm square indoor route that can be tested in simulation mode first.

## Text Command Editor

The program editor opens ordinary flight programs as concise text instead of raw Blockly XML:

```text
TAKE_OFF
SET_SPEED speed=3
REPEAT times=4 {
  WALK distance=50
  CHANGE_ANGLE angle=90
}
LAND
```

Distances and altitudes are centimeters, angles are degrees, and `WAIT` uses seconds. Saving converts the text back into a Blockly XML workspace that Drone Commander can reopen. The `?` button lists every supported command and parameter.

Programs containing advanced Blockly logic, variables, procedures, or non-literal expressions automatically open in the **Advanced XML** tab. The driver never performs a lossy text conversion.

## Build

Current platform:

```sh
make build
```

Linux AMD64:

```sh
make build-linux
```

Building Windows AMD64 from Linux requires MinGW-w64:

```sh
make build-windows
```

Building for macOS requires an Apple-compatible toolchain. On Linux, the target uses Docker, the `fyne-cross` image, and a macOS SDK extracted from Xcode or Command Line Tools:

```sh
make build-macos MACOS_SDK_PATH=/path/to/MacOSX.sdk
```

Fyne uses CGO to access native graphics APIs, so setting `GOOS` alone is not enough. Each target needs the appropriate C compiler and headers or SDK. On macOS, use `make build` to build for the local architecture.

## Command Mapping

| Drone Commander block | Tello command or behavior |
|---|---|
| Take off / Land | `takeoff` / `land` |
| Walk | `forward` or `back`, distance in cm |
| Slide | `right` or `left`, distance in cm |
| Set/change altitude | `up` or `down`, distance in cm |
| Go to / Move by | rotation and/or `go x y z speed` |
| Curve / Absolute curve | `curve x1 y1 z1 x2 y2 z2 speed`; compatible `go` fallback when the firmware curve limits are not met |
| Set/change angle | `cw` or `ccw` |
| Speed | Drone Commander range 0-10, mapped to 10-100 centimeters per second; default 3 = 30 cm/s (0 stops movement) |
| Wait | local delay with a keep-alive every 8 seconds |
| Smoke | ignored with a note in the flight log |

The driver maintains an estimated position to translate absolute movements. A standard Tello does not provide a global indoor position without external references, so wind, impacts, and drift can make `Go to`, `Absolute curve`, and `Return to base` inaccurate.

## Safety

- Test every program in simulation mode.
- Use propeller guards and completely clear the indoor flight area.
- Enter altitudes and distances in centimeters, remembering the Tello SDK minimum of 20 cm per linear command.
- Keep automatic landing enabled at the end of the program.
- **Stop / hovering** cancels the program and sends `stop`.
- **Land** cancels the program and sends `land`.
- **Emergency motor stop** sends `emergency`: the drone falls immediately.

Execution is limited to 10,000 blocks to stop non-terminating loops. Unsupported blocks are reported before flight.
Every flight action also writes a `STEP` analysis with duration and current telemetry to the persistent flight log.

## Options

```text
-tello 192.168.10.1:8889     UDP command address
-state :8890                 local telemetry bind address
-log PATH                   persistent flight log (default: user config directory)
```

## Project Structure

- `desktopui/` — native Fyne window and file selector;
- `session/` — application state, connection, and execution;
- `program/` — Blockly XML parser and interpreter;
- `flight/` — conversion from blocks to Tello commands;
- `tello/` — UDP protocol, telemetry, and offline simulator.

## Tests

```sh
go test ./...
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
