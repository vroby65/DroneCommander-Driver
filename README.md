# Drone Commander Tello Driver

A native Go desktop application that loads XML programs exported by [Drone Commander](https://github.com/vroby65/DroneCommander) and runs them on a Ryze/DJI Tello through Tello SDK 2.0. The interface is built with Fyne, opens as a regular desktop window, and does not start a web server or browser.

Drone Commander provides an [online editor and simulator](https://vroby65.github.io/DroneCommander/). Create and test a program in the browser, save it as an XML file, and open it in this driver for offline simulation or real flight.

## Drone Commander Workflow

1. Create the program in the [online editor](https://vroby65.github.io/DroneCommander/).
2. Test it in the 3D simulator and select **Save** to export an XML file.
3. Open the file in the driver and test it again in simulation mode.
4. Connect the computer to the `TELLO-...` Wi-Fi network and run the program on the drone.

## Downloads

Prebuilt packages are available from the [latest GitHub release](https://github.com/vroby65/DroneCommander-Driver/releases/latest):

| Platform | Package |
|---|---|
| Linux AMD64 | `DroneCommander-Driver-v1.2.0-linux-amd64.tar.gz` |
| Windows AMD64 | `DroneCommander-Driver-v1.2.0-windows-amd64.zip` |
| macOS Intel | `DroneCommander-Driver-v1.2.0-macos-amd64.tar.gz` |
| macOS Apple Silicon | `DroneCommander-Driver-v1.2.0-macos-arm64.tar.gz` |
| Android | `DroneCommander-Driver-v1.2.0-android-universal.apk` |

Use the supplied `SHA256SUMS` file to verify downloads. Desktop archives contain the executable, this README, and the MIT License. The macOS binaries are currently unsigned and not notarized; Gatekeeper may require manual approval on first launch. The Android APK is development-signed for direct sideloading and is not a Google Play package.

## Indoor Units

The conversion is fixed:

```text
1 Drone Commander unit = 1 centimeter
```

The Tello accepts the linear commands `up`, `down`, `left`, `right`, `forward`, and `back` only for distances from 20 to 500 cm. A `Walk 1` block therefore stops with an error; use `Walk 30` to move forward by 30 cm.

The `Return to base` block returns to the initial X/Z coordinates while maintaining the current altitude. This avoids reproducing the simulator's legacy altitude of 10 units, which would mean only 10 cm indoors.

The driver also rejects programmed flight altitudes below 20 cm. Use the `Land` block to descend below that threshold.

## Curved Flight

`Curve` uses coordinates relative to the current drone direction: X is right/left, Y is up/down, and Z is forward/backward. X/Y/Z identifies the intermediate point, while XD/YD/ZD is an offset from that intermediate point. `Absolute curve` instead uses two absolute program coordinates.

The Tello firmware accepts a native `curve` command only when the circular arc through the current position and the two requested points has a radius between 50 and 1,000 cm. The driver calculates that radius before sending the command. If the curve is outside the firmware limits, it records the fallback in the flight log and preserves the destination with compatible straight `go` segments; such a path will look more angular than the simulator curve. Use a radius of at least 50 cm when a smooth native curve is required.

## Run

Building requires Go 1.22, a C compiler, and the graphics libraries required by Fyne.

On Debian/Ubuntu:

```sh
sudo apt-get install gcc g++ ffmpeg libgl1-mesa-dev xorg-dev libxkbcommon-dev
git clone https://github.com/vroby65/DroneCommander-Driver.git
cd DroneCommander-Driver
go run .
```

In the application:

1. Choose the interface language. The driver supports the same languages as Drone Commander: English, Italiano, Français, Deutsch, Español, Português, العربية, 简体中文, 한국어, and 日本語. The system language is selected initially and the choice is remembered.
2. Choose an XML file saved by Drone Commander.
3. Enable simulation mode, connect, and test the program.
4. For real flight, connect the computer to the `TELLO-...` Wi-Fi network, disable simulation mode, and connect to the drone.
5. To see the live drone view, enable **Camera** below the lower-right preview. The control is available only with a real Tello connection.
6. Check the minimum battery threshold, choose whether to land at the end, optionally enable collision checking, and start the program.

When collision checking is enabled, the driver reads the latest ToF distance before translational movements. A reading below 30 cm blocks the movement, activates the normal error recovery and safety landing, and records the intervention in the flight log. The option is disabled by default.

## Camera, photos, and recordings

The camera receiver listens on UDP port 11111, sends the Tello SDK `streamon` / `streamoff` commands, and decodes the H.264 feed inside the application. FFmpeg is used when installed because it recovers more reliably from incomplete UDP frames produced by real-world Wi-Fi packet loss; an embedded OpenH264 decoder is retained as a fallback. The preview may remain on during a program run; its checkbox is locked until the run finishes to keep camera commands from interfering with a movement command.

The Drone Commander blocks **Take a photo**, **Start recording**, and **Save recording** use the manually enabled live camera. Photos are saved as timestamped PNG files. Recordings are encoded at 30 fps and saved as timestamped MP4 files when **Save recording** runs. An unfinished recording is discarded when the program stops or ends. Before every real run containing media blocks, the driver asks which folder should receive its photos and recordings; the current destination is also visible and changeable in the flight settings. `~/Pictures/DroneCommander` is only the initial suggestion and can be changed with `-media`.

Camera preview and photo/video capture are currently supported by the desktop builds. The Android build can run flight programs, but its H.264 camera decoder is not yet available.

FFmpeg is required to create MP4 recordings and is strongly recommended for the most resilient live preview. Make sure the `ffmpeg` executable is available in `PATH`:

- Debian/Ubuntu: `sudo apt-get install ffmpeg`
- macOS with Homebrew: `brew install ffmpeg`
- Windows: install an FFmpeg build and add its `bin` directory to `PATH`

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

Android packaging is experimental and uses `fyne-cross` with Docker to produce one sideload APK containing the supported mobile and emulator architectures:

```sh
go install github.com/fyne-io/fyne-cross@v1.6.2
make build-android
```

Install the resulting APK with `adb install`. The Android device must be connected to the Tello Wi-Fi network before using real flight mode. Camera preview and media capture are not currently available on Android.
This development APK is not a Google Play package; store distribution requires a persistent Android release keystore.

## Command Mapping

| Drone Commander block | Tello command or behavior |
|---|---|
| Take off / Land | `takeoff` / `land` |
| Take a photo | saves the latest decoded camera frame as a timestamped PNG |
| Start / Save recording | records decoded camera frames at 30 fps and saves a timestamped MP4 through FFmpeg |
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
- Use a curve radius of at least 50 cm for native smooth Tello curves.
- Keep automatic landing enabled at the end of the program.
- When collision checking is enabled, translational movements are blocked if the latest ToF distance is below 30 cm; both activation and intervention are written to the flight log.
- **Stop / hovering** cancels the program and sends `stop`.
- **Land** cancels the program and sends `land`.
- **Emergency motor stop** sends `emergency`: the drone falls immediately.

Execution is limited to 10,000 blocks to stop non-terminating loops. Unsupported blocks are reported before flight.
Every flight action also writes a `STEP` analysis with duration and current telemetry to the persistent flight log.

## Options

```text
-tello 192.168.10.1:8889     UDP command address
-state :8890                 local telemetry bind address
-video :11111                local camera video bind address
-log PATH                    persistent flight log (default: user config directory)
-media PATH                  initially suggested photo/recording directory
```

## Project Structure

- `desktopui/` — native Fyne window and file selector;
- `session/` — application state, connection, and execution;
- `program/` — Blockly XML parser and interpreter;
- `flight/` — conversion from blocks to Tello commands;
- `tello/` — UDP protocol, telemetry, H.264 camera receiver, and offline simulator.

## Tests

```sh
go test ./...
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
