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
	CentimetersPerUnit float64
	MinimumBattery     int
	KeyPressed         func(string) bool
	Log                func(string)
}

type Result struct {
	Flying  bool
	X, Y, Z float64
	Heading float64
}

const minimumIndoorAltitudeCM = 20.0

// Controller is a program.Host backed by a Tello commander.
type Controller struct {
	device    tello.Commander
	config    Config
	x, y, z   float64
	heading   float64
	speed     float64
	speedSent bool
	flying    bool
}

func NewController(device tello.Commander, config Config) *Controller {
	if config.CentimetersPerUnit <= 0 {
		config.CentimetersPerUnit = 1
	}
	return &Controller{device: device, config: config, speed: 1}
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

func (c *Controller) EnsureBattery(ctx context.Context) error {
	response, err := c.device.Command(ctx, "battery?")
	if err != nil {
		return fmt.Errorf("lettura batteria: %w", err)
	}
	battery, err := strconv.Atoi(strings.TrimSpace(response))
	if err != nil {
		return fmt.Errorf("risposta batteria non valida: %q", response)
	}
	if c.config.MinimumBattery > 0 && battery < c.config.MinimumBattery {
		return fmt.Errorf("batteria al %d%%, sotto la soglia di sicurezza del %d%%", battery, c.config.MinimumBattery)
	}
	c.log(fmt.Sprintf("Batteria: %d%%", battery))
	return nil
}

func (c *Controller) Action(ctx context.Context, kind string, arguments map[string]program.Value) error {
	n := func(name string) float64 { return numeric(arguments[name]) }
	switch kind {
	case "take_off":
		if c.flying {
			return nil
		}
		if err := c.EnsureBattery(ctx); err != nil {
			return err
		}
		if err := c.send(ctx, "takeoff"); err != nil {
			return err
		}
		c.flying = true
		if c.speed > 0 && !c.speedSent {
			if err := c.send(ctx, fmt.Sprintf("speed %d", c.speedCM(false))); err != nil {
				return err
			}
			c.speedSent = true
		}
		height := c.device.Snapshot().Height
		if height <= 0 {
			height = 80
		}
		c.y = float64(height) / c.config.CentimetersPerUnit
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
		return c.goTo(ctx, n("X"), n("Y"), n("Z"))
	case "return_to_base":
		if !c.flying || c.speed == 0 {
			return nil
		}
		// Indoors, returning at the current height is safer than reproducing the
		// simulator's legacy altitude of 10 units (now exactly 10 cm).
		return c.goTo(ctx, 0, c.y, 0)
	case "curve":
		if !c.flying || c.speed == 0 {
			return nil
		}
		return c.curveRelative(ctx, n("X"), n("Y"), n("Z"), n("XD"), n("YD"), n("ZD"))
	case "curve_abs":
		if !c.flying || c.speed == 0 {
			return nil
		}
		return c.curveAbsolute(ctx, n("X"), n("Y"), n("Z"), n("XD"), n("YD"), n("ZD"))
	case "wait":
		return c.wait(ctx, n("DIST"))
	case "set_speed":
		return c.setSpeed(ctx, n("SPEED"))
	case "smoke":
		c.log("Scia di fumo ignorata dal drone reale.")
		return nil
	default:
		return fmt.Errorf("azione drone sconosciuta: %s", kind)
	}
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
			return float64(height) / c.config.CentimetersPerUnit
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
	cmPerSecond := int(math.Round(value * c.config.CentimetersPerUnit))
	cmPerSecond = max(10, min(100, cmPerSecond))
	if err := c.send(ctx, fmt.Sprintf("speed %d", cmPerSecond)); err != nil {
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

func (c *Controller) directional(ctx context.Context, positive, negative string, units float64) error {
	if units == 0 {
		return nil
	}
	command := positive
	if units < 0 {
		command = negative
		units = -units
	}
	total := int(math.Round(units * c.config.CentimetersPerUnit))
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

// vector accepts Tello body coordinates: forward, right, and up.
func (c *Controller) vector(ctx context.Context, forward, right, up float64, curve bool) error {
	scale := c.config.CentimetersPerUnit
	target := [3]int{int(math.Round(forward * scale)), int(math.Round(right * scale)), int(math.Round(up * scale))}
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
	speed := int(math.Round(c.speed * c.config.CentimetersPerUnit))
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
		remaining -= float64(amount)
	}
	c.heading = normalize(c.heading + degrees)
	return nil
}

func (c *Controller) goTo(ctx context.Context, x, y, z float64) error {
	if err := c.validateAltitude(y); err != nil {
		return err
	}
	dx, dz := x-c.x, z-c.z
	horizontal := math.Hypot(dx, dz)
	if horizontal > 0 {
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
	scale := c.config.CentimetersPerUnit
	a := [3]int{}
	b := [3]int{}
	for axis := 0; axis < 3; axis++ {
		a[axis] = int(math.Round(via[axis] * scale))
		b[axis] = int(math.Round(target[axis] * scale))
		if abs(a[axis]) > 500 || abs(b[axis]) > 500 {
			return errors.New("le coordinate di una curva devono essere tra -500 e 500 cm")
		}
	}
	if max(abs(a[0]), max(abs(a[1]), abs(a[2]))) < 20 || max(abs(b[0]), max(abs(b[1]), abs(b[2]))) < 20 {
		return errors.New("ogni punto della curva deve distare almeno 20 cm")
	}
	radius, ok := circleRadius([3]float64{}, [3]float64{float64(a[0]), float64(a[1]), float64(a[2])}, [3]float64{float64(b[0]), float64(b[1]), float64(b[2])})
	if !ok {
		return errors.New("i punti della curva sono coincidenti o allineati")
	}
	if radius < 50 || radius > 1000 {
		return fmt.Errorf("raggio curva %.0f cm fuori dall'intervallo Tello 50-1000 cm", radius)
	}
	return c.send(ctx, fmt.Sprintf("curve %d %d %d %d %d %d %d", a[0], a[1], a[2], b[0], b[1], b[2], c.speedCM(true)))
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

func (c *Controller) validateAltitude(units float64) error {
	centimeters := units * c.config.CentimetersPerUnit
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
