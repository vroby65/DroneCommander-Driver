// Package flight translates Drone Commander actions into Tello SDK commands.
package flight

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/tello"
)

type Config struct {
	MinimumBattery int
	CollisionCheck bool
	KeyPressed     func(string) bool
	Log            func(string)
	TakePhoto      func(context.Context) (string, error)
	StartRecording func(context.Context) error
	SaveRecording  func(context.Context) (string, error)
}

type Result struct {
	Flying  bool
	X, Y, Z float64
	Heading float64
}

const (
	minimumIndoorAltitudeCM    = 20.0
	minimumCollisionDistanceCM = 30.0
)

// Controller is a program.Host backed by a Tello commander. Linear values read
// from Drone Commander XML remain centimeters and pass unchanged to the SDK.
type Controller struct {
	device      tello.Commander
	config      Config
	x, y, z     float64
	heading     float64
	speed       float64
	speedSent   bool
	flying      bool
	lastBattery int
}

const defaultSpeed = 3.0

func NewController(device tello.Commander, config Config) *Controller {
	return &Controller{device: device, config: config, speed: defaultSpeed}
}

func (c *Controller) Result() Result {
	return Result{Flying: c.flying, X: c.x, Y: c.y, Z: c.z, Heading: c.heading}
}

func (c *Controller) log(message string) {
	if c.config.Log != nil {
		c.config.Log(message)
	}
}

func (c *Controller) send(ctx context.Context, command string) error {
	c.log("→ " + command)
	response, err := c.device.Command(ctx, command)
	if err != nil {
		c.log("✕ " + err.Error())
		return err
	}
	c.log("← " + response)
	return nil
}

func (c *Controller) Battery(ctx context.Context) (int, error) {
	response, err := c.device.Command(ctx, "battery?")
	if err != nil {
		return 0, fmt.Errorf("lettura batteria: %w", err)
	}
	battery, err := strconv.Atoi(strings.TrimSpace(response))
	if err != nil {
		return 0, fmt.Errorf("risposta batteria non valida: %q", response)
	}
	return battery, nil
}

func (c *Controller) EnsureBattery(ctx context.Context) error {
	battery, err := c.Battery(ctx)
	if err != nil {
		return err
	}
	if c.config.MinimumBattery > 0 && battery < c.config.MinimumBattery {
		return fmt.Errorf("batteria al %d%%, sotto la soglia di sicurezza del %d%%", battery, c.config.MinimumBattery)
	}
	c.log(fmt.Sprintf("Batteria: %d%%", battery))
	return nil
}

func (c *Controller) Action(ctx context.Context, kind string, arguments map[string]program.Value) (err error) {
	started := time.Now()
	defer func() { c.logStep(kind, started, err) }()
	n := func(name string) float64 { return numeric(arguments[name]) }
	switch kind {
	case "take_off":
		if c.flying {
			return nil
		}
		if err := c.EnsureBattery(ctx); err != nil {
			return err
		}
		// The takeoff command may reach the drone even when its UDP response is
		// lost. Mark the flight as active before sending it so error recovery
		// still attempts to land.
		c.flying = true
		if err := c.send(ctx, "takeoff"); err != nil {
			return err
		}
		if c.speed > 0 && !c.speedSent {
			if err := c.send(ctx, fmt.Sprintf("speed %d", c.speedCM(false))); err != nil {
				return err
			}
			c.speedSent = true
		}
		state := c.device.Snapshot()
		height := state.Height
		if height <= 0 {
			height = 80
		}
		c.y = float64(height)
		// A controller is recreated for every run, but the real drone keeps its
		// orientation after landing. Start absolute angles from the live yaw.
		c.heading = normalize(float64(state.Yaw))
		return nil
	case "land":
		if !c.flying {
			return nil
		}
		if err := c.send(ctx, "land"); err != nil {
			return err
		}
		c.flying, c.y = false, 0
		return nil
	case "take_photo":
		if c.config.TakePhoto == nil {
			c.log("Foto non disponibile: il gestore della telecamera non è configurato.")
			return nil
		}
		path, photoErr := c.config.TakePhoto(ctx)
		if photoErr != nil {
			c.log("Foto non riuscita: " + photoErr.Error())
			return nil
		}
		c.log("Foto salvata: " + path)
		return nil
	case "start_recording":
		if c.config.StartRecording == nil {
			c.log("Registrazione non disponibile: il gestore della telecamera non è configurato.")
			return nil
		}
		if recordingErr := c.config.StartRecording(ctx); recordingErr != nil {
			c.log("Registrazione non avviata: " + recordingErr.Error())
			return nil
		}
		c.log("Registrazione video avviata.")
		return nil
	case "save_recording":
		if c.config.SaveRecording == nil {
			c.log("Salvataggio registrazione non disponibile: il gestore della telecamera non è configurato.")
			return nil
		}
		path, recordingErr := c.config.SaveRecording(ctx)
		if recordingErr != nil {
			c.log("Registrazione non salvata: " + recordingErr.Error())
			return nil
		}
		c.log("Registrazione video salvata: " + path)
		return nil
	case "set_altitude":
		if !c.flying {
			return nil
		}
		return c.changeHeight(ctx, n("ALTITUDE")-c.y)
	case "change_altitude":
		if !c.flying {
			return nil
		}
		return c.changeHeight(ctx, n("ALTITUDE"))
	case "set_angle":
		if !c.flying {
			return nil
		}
		target := normalize(n("ANGLE"))
		return c.turn(ctx, shortest(c.heading, target))
	case "change_angle":
		if !c.flying {
			return nil
		}
		return c.turn(ctx, n("ANGLE"))
	case "walk":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		distance := n("DIST")
		if err := c.directional(ctx, "forward", "back", distance); err != nil {
			return err
		}
		c.advance(0, distance, 0)
		return nil
	case "slide":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		distance := n("SLIDE")
		if err := c.directional(ctx, "right", "left", distance); err != nil {
			return err
		}
		c.advance(distance, 0, 0)
		return nil
	case "walk_climbing":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		forward, up := n("DIST"), n("CLIMB")
		if err := c.validateAltitude(c.y + up); err != nil {
			return err
		}
		if err := c.vector(ctx, forward, 0, up, false); err != nil {
			return err
		}
		c.advance(0, forward, up)
		return nil
	case "move_by":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		right, up, forward := n("X"), n("Y"), n("Z")
		if err := c.validateAltitude(c.y + up); err != nil {
			return err
		}
		if err := c.vector(ctx, forward, right, up, false); err != nil {
			return err
		}
		c.advance(right, forward, up)
		return nil
	case "go_to":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		return c.goTo(ctx, n("X"), n("Y"), n("Z"))
	case "return_to_base":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		// Indoors, returning at the current height is safer than reproducing the
		// simulator's legacy altitude of 10 units (now exactly 10 cm).
		return c.goTo(ctx, 0, c.y, 0)
	case "curve":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		return c.curveRelative(ctx, n("X"), n("Y"), n("Z"), n("XD"), n("YD"), n("ZD"))
	case "curve_abs":
		if !c.flying || c.speed == 0 {
			return nil
		}
		if err := c.checkCollision(); err != nil {
			return err
		}
		return c.curveAbsolute(ctx, n("X"), n("Y"), n("Z"), n("XD"), n("YD"), n("ZD"))
	case "wait":
		return c.wait(ctx, n("DIST"))
	case "set_speed":
		return c.setSpeed(ctx, n("SPEED"))
	case "smoke":
		c.log("Scia di fumo ignorata dal drone reale.")
		return nil
	case "text_print":
		c.log("Stampa: " + fmt.Sprint(arguments["TEXT"]))
		return nil
	default:
		return fmt.Errorf("azione drone sconosciuta: %s", kind)
	}
}

func (c *Controller) checkCollision() error {
	if !c.config.CollisionCheck {
		return nil
	}
	distance, available := c.device.Snapshot().Values["tof"]
	if !available || distance <= 0 || distance >= minimumCollisionDistanceCM {
		return nil
	}
	err := fmt.Errorf("ostacolo rilevato a %.0f cm (distanza minima %.0f cm)", distance, minimumCollisionDistanceCM)
	c.log("CONTROLLO COLLISIONI: " + err.Error() + "; movimento bloccato.")
	return err
}

func (c *Controller) logStep(kind string, started time.Time, actionErr error) {
	state := c.device.Snapshot()
	status := "OK"
	var findings []string
	if actionErr != nil {
		status = "ERRORE"
		findings = append(findings, actionErr.Error())
	}
	if state.Battery > 0 {
		if state.Battery <= 30 {
			findings = append(findings, fmt.Sprintf("batteria bassa %d%%", state.Battery))
		}
		if c.lastBattery > 0 && c.lastBattery-state.Battery >= 5 {
			findings = append(findings, fmt.Sprintf("calo batteria %d%%→%d%%", c.lastBattery, state.Battery))
		}
		c.lastBattery = state.Battery
	}
	if !state.UpdatedAt.IsZero() && time.Since(state.UpdatedAt) > 3*time.Second {
		findings = append(findings, "telemetria non aggiornata da oltre 3 s")
	}
	if abs(state.Pitch) > 20 || abs(state.Roll) > 20 {
		findings = append(findings, fmt.Sprintf("assetto marcato pitch=%d roll=%d", state.Pitch, state.Roll))
	}
	analysis := "analisi OK"
	if len(findings) > 0 {
		analysis = "analisi: " + strings.Join(findings, "; ")
	}
	details := fmt.Sprintf("STEP %s · %s · %s", kind, status, time.Since(started).Round(100*time.Millisecond))
	if state.Battery > 0 {
		details += fmt.Sprintf(" · batteria %d%%", state.Battery)
	}
	details += fmt.Sprintf(" · quota %d cm · yaw %d° · %s", state.Height, state.Yaw, analysis)
	c.log(details)
}

func (c *Controller) Sensor(name, argument string) program.Value {
	switch name {
	case "key":
		if c.config.KeyPressed != nil {
			return c.config.KeyPressed(argument)
		}
		return false
	case "x":
		return c.x
	case "z":
		return c.z
	case "altitude":
		height := c.device.Snapshot().Height
		if height > 0 {
			return float64(height)
		}
		return c.y
	case "direction":
		return c.heading
	case "speed":
		return c.speed
	}
	return float64(0)
}

func (c *Controller) changeHeight(ctx context.Context, delta float64) error {
	if err := c.validateAltitude(c.y + delta); err != nil {
		return err
	}
	if delta == 0 {
		return nil
	}
	if err := c.directional(ctx, "up", "down", delta); err != nil {
		return err
	}
	c.y += delta
	return nil
}

func (c *Controller) setSpeed(ctx context.Context, value float64) error {
	value = math.Max(0, math.Min(10, value))
	c.speed = value
	if value == 0 {
		c.log("Velocita impostata a zero: i movimenti saranno saltati.")
		return nil
	}
	if err := c.send(ctx, fmt.Sprintf("speed %d", c.speedCM(false))); err != nil {
		return err
	}
	c.speedSent = true
	return nil
}

func (c *Controller) wait(ctx context.Context, seconds float64) error {
	if seconds <= 0 {
		return nil
	}
	remaining := time.Duration(seconds * float64(time.Second))
	c.log(fmt.Sprintf("Attesa %.2f s", seconds))
	for remaining > 0 {
		chunk := min(remaining, 8*time.Second)
		timer := time.NewTimer(chunk)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		remaining -= chunk
		if remaining > 0 {
			if err := c.send(ctx, "command"); err != nil {
				return fmt.Errorf("keep-alive durante l'attesa: %w", err)
			}
		}
	}
	return nil
}

func (c *Controller) directional(ctx context.Context, positive, negative string, centimeters float64) error {
	if centimeters == 0 {
		return nil
	}
	command := positive
	if centimeters < 0 {
		command = negative
		centimeters = -centimeters
	}
	total := int(math.Round(centimeters))
	parts, err := splitDistance(total)
	if err != nil {
		return fmt.Errorf("movimento %s: %w", command, err)
	}
	for _, distance := range parts {
		if err := c.send(ctx, fmt.Sprintf("%s %d", command, distance)); err != nil {
			return err
		}
	}
	return nil
}

func splitDistance(total int) ([]int, error) {
	if total < 20 {
		return nil, fmt.Errorf("%d cm: il minimo Tello e 20 cm", total)
	}
	count := int(math.Ceil(float64(total) / 500))
	parts := make([]int, count)
	previous := 0
	for index := 1; index <= count; index++ {
		current := int(math.Round(float64(total*index) / float64(count)))
		parts[index-1] = current - previous
		previous = current
		if parts[index-1] < 20 || parts[index-1] > 500 {
			return nil, fmt.Errorf("segmento fuori intervallo 20-500 cm")
		}
	}
	return parts, nil
}

// vector accepts Drone Commander body coordinates: forward, right, and up.
// Tello's go/curve lateral axis is positive to the left, so right is negated
// only when the command is encoded for the SDK.
func (c *Controller) vector(ctx context.Context, forward, right, up float64, curve bool) error {
	target := sdkVector(forward, right, up)
	largest := max(abs(target[0]), max(abs(target[1]), abs(target[2])))
	if largest == 0 {
		return nil
	}
	if largest < 20 {
		return fmt.Errorf("movimento di %d cm: almeno un asse deve raggiungere 20 cm", largest)
	}
	count := int(math.Ceil(float64(largest) / 500))
	previous := [3]int{}
	for part := 1; part <= count; part++ {
		current := [3]int{}
		for axis := range current {
			current[axis] = int(math.Round(float64(target[axis]*part) / float64(count)))
		}
		delta := [3]int{current[0] - previous[0], current[1] - previous[1], current[2] - previous[2]}
		previous = current
		segmentMax := max(abs(delta[0]), max(abs(delta[1]), abs(delta[2])))
		if segmentMax < 20 {
			return errors.New("un segmento del movimento e inferiore a 20 cm")
		}
		speed := c.speedCM(curve)
		if err := c.send(ctx, fmt.Sprintf("go %d %d %d %d", delta[0], delta[1], delta[2], speed)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) speedCM(curve bool) int {
	// Drone Commander exposes an abstract 0-10 speed, while the Tello SDK
	// expects centimeters per second in the 10-100 range.
	speed := int(math.Round(c.speed * 10))
	speed = max(10, min(100, speed))
	if curve {
		speed = min(60, speed)
	}
	return speed
}

func (c *Controller) turn(ctx context.Context, degrees float64) error {
	if degrees == 0 {
		return nil
	}
	clockwise := degrees > 0
	remaining := math.Abs(degrees)
	executed := 0.0
	for remaining >= 0.5 {
		amount := min(360, int(math.Round(remaining)))
		if amount < 1 {
			break
		}
		command := "ccw"
		if clockwise {
			command = "cw"
		}
		if err := c.send(ctx, fmt.Sprintf("%s %d", command, amount)); err != nil {
			return err
		}
		executed += float64(amount)
		remaining -= float64(amount)
	}
	if !clockwise {
		executed = -executed
	}
	c.heading = normalize(c.heading + executed)
	return nil
}

func (c *Controller) goTo(ctx context.Context, x, y, z float64) error {
	if err := c.validateAltitude(y); err != nil {
		return err
	}
	dx, dz := x-c.x, z-c.z
	horizontal := math.Hypot(dx, dz)
	// Floating-point residue after a closed route must not rotate the drone in
	// an arbitrary direction when the rounded displacement is actually zero.
	if math.Round(horizontal) != 0 {
		target := normalize(math.Atan2(dx, dz) * 180 / math.Pi)
		if err := c.turn(ctx, shortest(c.heading, target)); err != nil {
			return err
		}
	}
	if err := c.vector(ctx, horizontal, 0, y-c.y, false); err != nil {
		return err
	}
	c.x, c.y, c.z = x, y, z
	return nil
}

func (c *Controller) curveRelative(ctx context.Context, x, y, z, xd, yd, zd float64) error {
	if err := c.validateAltitude(c.y + y); err != nil {
		return fmt.Errorf("punto intermedio: %w", err)
	}
	if err := c.validateAltitude(c.y + y + yd); err != nil {
		return fmt.Errorf("punto finale: %w", err)
	}
	// Drone Commander supplies (right, up, forward); the second point is relative to the first.
	via := [3]float64{z, x, y}
	target := [3]float64{z + zd, x + xd, y + yd}
	if err := c.curveCommand(ctx, via, target); err != nil {
		return err
	}
	c.advance(target[1], target[0], target[2])
	return nil
}

func (c *Controller) curveAbsolute(ctx context.Context, x, y, z, xd, yd, zd float64) error {
	if err := c.validateAltitude(y); err != nil {
		return fmt.Errorf("punto intermedio: %w", err)
	}
	if err := c.validateAltitude(yd); err != nil {
		return fmt.Errorf("punto finale: %w", err)
	}
	viaRight, viaForward := c.toBody(x-c.x, z-c.z)
	targetRight, targetForward := c.toBody(xd-c.x, zd-c.z)
	if err := c.curveCommand(ctx, [3]float64{viaForward, viaRight, y - c.y}, [3]float64{targetForward, targetRight, yd - c.y}); err != nil {
		return err
	}
	c.x, c.y, c.z = xd, yd, zd
	return nil
}

func (c *Controller) curveCommand(ctx context.Context, via, target [3]float64) error {
	viaCM := roundedVector(via)
	targetCM := roundedVector(target)
	a := sdkVector(via[0], via[1], via[2])
	b := sdkVector(target[0], target[1], target[2])
	for axis := 0; axis < 3; axis++ {
		if abs(a[axis]) > 500 || abs(b[axis]) > 500 {
			return errors.New("le coordinate di una curva devono essere tra -500 e 500 cm")
		}
	}
	pointsReachMinimum := vectorMagnitude(a) >= 20 && vectorMagnitude(b) >= 20
	radius, ok := circleRadius([3]float64{}, [3]float64{float64(a[0]), float64(a[1]), float64(a[2])}, [3]float64{float64(b[0]), float64(b[1]), float64(b[2])})
	if pointsReachMinimum && ok && radius >= 50 && radius <= 1000 {
		return c.send(ctx, fmt.Sprintf("curve %d %d %d %d %d %d %d", a[0], a[1], a[2], b[0], b[1], b[2], c.speedCM(true)))
	}

	// The SDK rejects curves whose points are shorter than 20 cm, collinear,
	// or outside its 50-1000 cm radius. Preserve the requested destination by
	// approximating the path with two straight go commands when possible.
	c.log("Curva non accettata nativamente dal Tello; uso segmenti lineari compatibili.")
	second := [3]int{targetCM[0] - viaCM[0], targetCM[1] - viaCM[1], targetCM[2] - viaCM[2]}
	if vectorMagnitude(viaCM) >= 20 && vectorMagnitude(second) >= 20 {
		if err := c.vector(ctx, float64(viaCM[0]), float64(viaCM[1]), float64(viaCM[2]), false); err != nil {
			return fmt.Errorf("primo segmento sostitutivo della curva: %w", err)
		}
		if err := c.vector(ctx, float64(second[0]), float64(second[1]), float64(second[2]), false); err != nil {
			return fmt.Errorf("secondo segmento sostitutivo della curva: %w", err)
		}
		return nil
	}
	if vectorMagnitude(targetCM) >= 20 {
		c.log("I punti della curva sono troppo vicini; raggiungo direttamente la destinazione.")
		return c.vector(ctx, float64(targetCM[0]), float64(targetCM[1]), float64(targetCM[2]), false)
	}
	return errors.New("curva inferiore a 20 cm: il Tello non puo eseguire neppure il percorso sostitutivo")
}

func roundedVector(vector [3]float64) [3]int {
	return [3]int{int(math.Round(vector[0])), int(math.Round(vector[1])), int(math.Round(vector[2]))}
}

func sdkVector(forward, right, up float64) [3]int {
	return roundedVector([3]float64{forward, -right, up})
}

func vectorMagnitude(vector [3]int) int {
	return max(abs(vector[0]), max(abs(vector[1]), abs(vector[2])))
}

func circleRadius(a, b, c [3]float64) (float64, bool) {
	distance := func(p, q [3]float64) float64 {
		return math.Sqrt((p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1]) + (p[2]-q[2])*(p[2]-q[2]))
	}
	ab, bc, ca := distance(a, b), distance(b, c), distance(c, a)
	u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	cross := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
	twiceArea := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
	if twiceArea < 1e-9 {
		return 0, false
	}
	return ab * bc * ca / (2 * twiceArea), true
}

func (c *Controller) advance(right, forward, up float64) {
	radians := c.heading * math.Pi / 180
	c.x += right*math.Cos(radians) + forward*math.Sin(radians)
	c.z += -right*math.Sin(radians) + forward*math.Cos(radians)
	c.y += up
}

func (c *Controller) validateAltitude(centimeters float64) error {
	if centimeters < minimumIndoorAltitudeCM {
		return fmt.Errorf("quota indoor %.0f cm: il minimo di sicurezza e %.0f cm; usa Atterra per scendere", centimeters, minimumIndoorAltitudeCM)
	}
	return nil
}
func (c *Controller) toBody(dx, dz float64) (right, forward float64) {
	radians := c.heading * math.Pi / 180
	return dx*math.Cos(radians) - dz*math.Sin(radians), dx*math.Sin(radians) + dz*math.Cos(radians)
}
func numeric(value program.Value) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case bool:
		if v {
			return 1
		}
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	}
	return 0
}
func normalize(value float64) float64   { return math.Mod(math.Mod(value, 360)+360, 360) }
func shortest(from, to float64) float64 { return math.Mod(to-from+540, 360) - 180 }
func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
