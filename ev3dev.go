// Copyright ©2016 The ev3go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ev3dev provides low level access to the ev3dev control and sensor drivers.
// See documentation at http://www.ev3dev.org/docs/drivers/.
//
// The API provided in the ev3dev package allows fluent chaining of action calls.
// Methods for each of the device handle types are split into two classes: action
// and result. Action method calls return the receiver and result method calls
// return an error value generally with another result. Action methods result in
// a change of state in the robot while result methods return the requested attribute
// state of the robot.
//
// To allow fluent call chains, errors are sticky for action methods and are cleared
// and returned by result methods. In a chain of calls the first error that is caused
// by an action method prevents execution of all subsequent action method calls, and
// is returned by the first result method called on the device handle, clearing the
// error state. Any attribute value returned by a call chain returning a non-nil error
// is invalid.
package ev3dev

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	linearPrefix = "linear"
	motorPrefix  = "motor"
	portPrefix   = "port"
	sensorPrefix = "sensor"
)

// prefix is the filesystem root prefix.
// Currently it is used only for testing.
var prefix = ""

const (
	// LEDPath is the path to the ev3 LED file system.
	LEDPath = "/sys/class/leds"

	// ButtonPath is the path to the ev3 button events.
	ButtonPath = "/dev/input/by-path/platform-gpio-keys.0-event"

	// LegoPortPath is the path to the ev3 lego-port file system.
	LegoPortPath = "/sys/class/lego-port"

	// SensorPath is the path to the ev3 lego-sensor file system.
	SensorPath = "/sys/class/lego-sensor"

	// TachoMotorPath is the path to the ev3 tacho-motor file system.
	TachoMotorPath = "/sys/class/tacho-motor"

	// ServoMotorPath is the path to the ev3 servo-motor file system.
	ServoMotorPath = "/sys/class/servo-motor"

	// DCMotorPath is the path to the ev3 dc-motor file system.
	DCMotorPath = "/sys/class/dc-motor"

	// PowerSupplyPath is the path to the ev3 power supply file system.
	PowerSupplyPath = "/sys/class/power_supply"
)

// These are the subsystem path definitions for all device classes.
const (
	address                   = "address"
	binData                   = "bin_data"
	batteryTechnology         = "technology"
	batteryType               = "type"
	binDataFormat             = "bin_data_format"
	brightness                = "brightness"
	command                   = "command"
	commands                  = "commands"
	countPerMeter             = "count_per_m"
	countPerRot               = "count_per_rot"
	currentNow                = "current_now"
	decimals                  = "decimals"
	delayOff                  = "delay_off"
	delayOn                   = "delay_on"
	direct                    = "direct"
	driverName                = "driver_name"
	dutyCycle                 = "duty_cycle"
	dutyCycleSetpoint         = "duty_cycle_sp"
	fullTravelCount           = "full_travel_count"
	holdPID                   = "hold_pid"
	holdPIDkd                 = holdPID + "/" + kd
	holdPIDki                 = holdPID + "/" + ki
	holdPIDkp                 = holdPID + "/" + kp
	kd                        = "Kd"
	ki                        = "Ki"
	kp                        = "Kp"
	maxBrightness             = "max_brightness"
	maxPulseSetpoint          = "max_pulse_sp"
	midPulseSetpoint          = "mid_pulse_sp"
	minPulseSetpoint          = "min_pulse_sp"
	maxSpeed                  = "max_speed"
	mode                      = "mode"
	modes                     = "modes"
	numValues                 = "num_values"
	polarity                  = "polarity"
	pollRate                  = "poll_ms"
	position                  = "position"
	positionSetpoint          = "position_sp"
	power                     = "power"
	powerAutosuspendDelay     = power + "/" + "autosuspend_delay_ms"
	powerControl              = power + "/" + "control"
	powerRuntimeActiveTime    = power + "/" + "runtime_active_time"
	powerRuntimeStatus        = power + "/" + "runtime_status"
	powerRuntimeSuspendedTime = power + "/" + "runtime_suspended_time"
	rampDownSetpoint          = "ramp_down_sp"
	rampUpSetpoint            = "ramp_up_sp"
	rateSetpoint              = "rate_sp"
	setDevice                 = "set_device"
	speed                     = "speed"
	speedPID                  = "speed_pid"
	speedPIDkd                = speedPID + "/" + kd
	speedPIDki                = speedPID + "/" + ki
	speedPIDkp                = speedPID + "/" + kp
	speedSetpoint             = "speed_sp"
	state                     = "state"
	status                    = "status"
	stopAction                = "stop_action"
	stopActions               = "stop_actions"
	subsystem                 = "subsystem"
	textValues                = "text_values"
	timeSetpoint              = "time_sp"
	trigger                   = "trigger"
	uevent                    = "uevent"
	units                     = "units"
	value                     = "value"
	voltageMaxDesign          = "voltage_max_design"
	voltageMinDesign          = "voltage_min_design"
	voltageNow                = "voltage_now"
)

// Polarity represent motor polarity states.
type Polarity string

const (
	Normal   Polarity = "normal"
	Inversed Polarity = "inversed"
)

// MotorState is a flag set representing the state of a TachoMotor.
type MotorState uint

const (
	Running MotorState = 1 << iota
	Ramping
	Holding
	Overloaded
	Stalled
)

const (
	running    = "running"
	ramping    = "ramping"
	holding    = "holding"
	overloaded = "overloaded"
	stalled    = "stalled"
)

var motorStateTable = map[string]MotorState{
	running:    Running,
	ramping:    Ramping,
	holding:    Holding,
	overloaded: Overloaded,
	stalled:    Stalled,
}

func keys(states map[string]MotorState) []string {
	l := make([]string, 0, len(states))
	for k := range states {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}

var motorStates = []string{
	running,
	ramping,
	holding,
	overloaded,
	stalled,
}

// String satisfies the fmt.Stringer interface.
func (f MotorState) String() string {
	const stateMask = Running | Ramping | Holding | Overloaded | Stalled

	var b []byte
	for i, s := range motorStates {
		if f&(1<<uint(i)) != 0 {
			if len(b) != 0 {
				b = append(b, '|')
			}
			b = append(b, s...)
		}
	}
	if b == nil {
		return "none"
	}
	return string(b)
}

// StaterDevice is a device that can return a motor state.
type StaterDevice interface {
	Device
	State() (MotorState, error)
}

var canPoll = true

// Wait blocks until the wanted motor state under the motor state mask is
// reached, or the timeout is reached. If timeout is negative Wait will wait
// indefinitely for the wanted motor state has been reached.
// The last unmasked motor state is returned unless the timeout was reached
// before the motor state was read.
// When the any parameter is false, Wait will return ok as true if
//  (state&mask)^not == want|not
// and when any is true Wait return false if
//  (state&mask)^not != 0 && state&mask&not == 0 .
// Otherwise ok will return false indicating that the returned state did
// not match the request.
// Wait will not set the error state of the StaterDevice, but will clear and
// return it if it is not nil.
func Wait(d StaterDevice, mask, want, not MotorState, any bool, timeout time.Duration) (stat MotorState, ok bool, err error) {
	// We use a direct implementation of the State method here
	// to ensure we are polling on the same file as we are reading
	// from. Also, since we are potentially probing the state
	// repeatedly, we save file opens.
	//
	// This also allows us to test the code, which would not
	// otherwise be possible since sysiphus cannot do POLLPRI
	// polling, due to limitations in FUSE.

	// Check if we can proceed.
	err = d.Err()
	if err != nil {
		return 0, false, err
	}

	path := filepath.Join(d.Path(), d.String(), state)
	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	// See if we can exit early.
	stat, err = motorState(d, f)
	if err != nil {
		return stat, false, err
	}
	if stateIsOK(stat, mask, want, not, any) {
		return stat, true, nil
	}

	var fds []unix.PollFd
	if canPoll {
		fds = []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}

		// Read a single byte to mark f as unchanged.
		f.ReadAt([]byte{0}, 0)
	}

	end := time.Now().Add(timeout)
	for timeout < 0 || time.Since(end) < 0 {
		if canPoll {
			_timeout := timeout
			if timeout >= 0 {
				if remain := end.Sub(time.Now()); remain < timeout {
					_timeout = remain
				}
			}
			n, err := unix.Poll(fds, int(_timeout/time.Millisecond))
			if n == 0 {
				return 0, false, err
			}
		}
		stat, err = motorState(d, f)
		if err != nil {
			return stat, false, err
		}
		if stateIsOK(stat, mask, want, not, any) {
			return stat, true, nil
		}

		relax := 50 * time.Millisecond
		if remain := end.Sub(time.Now()); remain < relax {
			relax = remain / 2
		}
		time.Sleep(relax)
	}

	return stat, false, nil
}

func motorState(d Device, f *os.File) (MotorState, error) {
	var b [4096]byte
	n, err := f.ReadAt(b[:], 0)
	if n == len(b) && err == nil {
		// This is more strict that justified by the
		// io.ReaderAt docs, but we prefer failure
		// and a short buffer is extremely unlikely.
		return 0, errors.New("ev3dev: buffer full")
	}
	if err == io.EOF {
		err = nil
	}
	return stateFrom(d, string(chomp(b[:n])), "", err)
}

func stateIsOK(stat, mask, want, not MotorState, any bool) bool {
	if any {
		stat &= mask
		return stat^not != 0 && stat&not == 0
	}
	return (stat&mask)^not == want|not
}

// DriverMismatch errors are returned when a device is found that
// does not match the requested driver.
type DriverMismatch struct {
	// Want is the string describing
	// the requested driver.
	Want string

	// Have is the string describing
	// the driver present on the device.
	Have string
}

func (e DriverMismatch) Error() string {
	return fmt.Sprintf("ev3dev: mismatched driver names: want %q but have %q", e.Want, e.Have)
}

// Device is an ev3dev API device.
type Device interface {
	// Path returns the sysfs path
	// for the device type.
	Path() string

	// Type returns the type of the
	// device, one of "linear", "motor",
	// "port" or "sensor".
	Type() string

	// Err returns and clears the
	// error state of the Device.
	Err() error

	fmt.Stringer
}

// idSetter is a Device that can set its
// device id and clear its error field.
type idSetter interface {
	Device

	// idInt returns the id integer value of
	// the device.
	// A nil idSetter must return -1.
	idInt() int

	// setID sets the device id to the given id
	// and clears the error field to allow an
	// already used Device to be reused.
	setID(id int)
}

// FindAfter finds the first device after d matching the class of the
// dst Device with the given driver name, or returns an error. The
// concrete types of d and dst must match. On return with a nil
// error, dst is usable as a handle for the device.
// If d is nil, FindAfter finds the first matching device.
//
// Only ev3dev.Device implementations are supported.
func FindAfter(d, dst Device, driver string) error {
	_, ok := dst.(idSetter)
	if !ok {
		return fmt.Errorf("ev3dev: device type %T not supported", dst)
	}

	after := -1
	if d != nil {
		if reflect.TypeOf(d) != reflect.TypeOf(dst) {
			return fmt.Errorf("ev3dev: device types do not match %T != %T", d, dst)
		}
		after = d.(idSetter).idInt()
	}

	id, err := deviceIDFor("", driver, dst, after)
	if err != nil {
		return err
	}
	dst.(idSetter).setID(id)
	return nil
}

// IsConnected returns whether the Device is connected.
func IsConnected(d Device) (ok bool, err error) {
	_, err = os.Stat(fmt.Sprintf(d.Path()+"/%s", d))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		err = nil
	}
	return false, err
}

// AddressOf returns the port address of the Device.
func AddressOf(d Device) (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(d.Path()+"/%s/"+address, d))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read %s address: %v", d.Type(), err)
	}
	return string(chomp(b)), err
}

// DriverFor returns the driver name for the Device.
func DriverFor(d Device) (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(d.Path()+"/%s/"+driverName, d))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read %s driver name: %v", d.Type(), err)
	}
	return string(chomp(b)), err
}

// deviceIDFor returns the id for the given ev3 port name and driver of the Device.
// If the driver does not match the driver string, an id for the device is returned
// with a DriverMismatch error.
// If port is empty, the first device satisfying the driver name with an id after the
// specified after parameter is returned.
func deviceIDFor(port, driver string, d Device, after int) (int, error) {
	devNames, err := devicesIn(d.Path())
	if err != nil {
		return -1, fmt.Errorf("ev3dev: could not get devices for %s: %v", d.Path(), err)
	}
	devices, err := sortedDevices(devNames, d.Type())
	if err != nil {
		return -1, err
	}

	portBytes := []byte(port)
	driverBytes := []byte(driver)
	for _, device := range devices {
		if port == "" {
			if device.id <= after {
				continue
			}
			path := filepath.Join(d.Path(), device.name, driverName)
			b, err := ioutil.ReadFile(path)
			if os.IsNotExist(err) {
				// If the device disappeared
				// try the next one.
				continue
			}
			if err != nil {
				return -1, fmt.Errorf("ev3dev: could not read driver name %s: %v", path, err)
			}
			if !bytes.Equal(driverBytes, chomp(b)) {
				continue
			}
			return device.id, nil
		}

		path := filepath.Join(d.Path(), device.name, address)
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return -1, fmt.Errorf("ev3dev: could not read address %s: %v", path, err)
		}
		if !bytes.Equal(portBytes, chomp(b)) {
			continue
		}
		path = filepath.Join(d.Path(), device.name, driverName)
		b, err = ioutil.ReadFile(path)
		if err != nil {
			return -1, fmt.Errorf("ev3dev: could not read driver name %s: %v", path, err)
		}
		if !bytes.Equal(driverBytes, chomp(b)) {
			err = DriverMismatch{Want: driver, Have: string(chomp(b))}
		}
		return device.id, err
	}

	if port != "" {
		return -1, fmt.Errorf("ev3dev: could not find device for driver %q on port %s", driver, port)
	}
	if after < 0 {
		return -1, fmt.Errorf("ev3dev: could not find device for driver %q", driver)
	}
	return -1, fmt.Errorf("ev3dev: could find device with driver name %q after %s%d", driver, d.Type(), after)
}

func devicesIn(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(0)
}

func sortedDevices(names []string, prefix string) ([]idDevice, error) {
	devices := make([]idDevice, 0, len(names))
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		id, err := strconv.Atoi(n[len(prefix):])
		if err != nil {
			return nil, fmt.Errorf("ev3dev: could not parse id from device name %q: %v", n, err)
		}
		devices = append(devices, idDevice{id: id, name: n})
	}
	sort.Sort(byID(devices))
	return devices, nil
}

type idDevice struct {
	id   int
	name string
}

type byID []idDevice

func (d byID) Len() int           { return len(d) }
func (d byID) Less(i, j int) bool { return d[i].id < d[j].id }
func (d byID) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func attributeOf(d Device, attr string) (dev Device, data string, _attr string, err error) {
	err = d.Err()
	if err != nil {
		return d, "", "", err
	}
	path := filepath.Join(d.Path(), d.String(), attr)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return d, "", "", newAttrOpError(d, attr, string(b), "read", err)
	}
	return d, string(chomp(b)), attr, nil
}

func chomp(b []byte) []byte {
	if b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

func intFrom(d Device, data, attr string, err error) (int, error) {
	if err != nil {
		return -1, err
	}
	i, err := strconv.Atoi(data)
	if err != nil {
		return -1, newParseError(d, attr, err)
	}
	return i, nil
}

func float64From(d Device, data, attr string, err error) (float64, error) {
	if err != nil {
		return math.NaN(), err
	}
	f, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return math.NaN(), newParseError(d, attr, err)
	}
	return f, nil
}

func durationFrom(dev Device, data, attr string, err error) (time.Duration, error) {
	if err != nil {
		return -1, err
	}
	d, err := strconv.Atoi(data)
	if err != nil {
		return -1, newParseError(dev, attr, err)
	}
	return time.Duration(d) * time.Millisecond, nil
}

func stringFrom(_ Device, data, _ string, err error) (string, error) {
	return data, err
}

func stringSliceFrom(_ Device, data, _ string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	return strings.Split(data, " "), nil
}

func stateFrom(d Device, data, _ string, err error) (MotorState, error) {
	if err != nil {
		return 0, err
	}
	if data == "" {
		return 0, nil
	}
	var stat MotorState
	for _, s := range strings.Split(data, " ") {
		bit, ok := motorStateTable[s]
		if !ok {
			return 0, newInvalidValueError(d, state, "unrecognized motor state", s, keys(motorStateTable))
		}
		stat |= bit
	}
	return stat, nil
}

func ueventFrom(d Device, data, attr string, err error) (map[string]string, error) {
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	uevent := make(map[string]string)
	for _, l := range strings.Split(data, "\n") {
		parts := strings.Split(l, "=")
		if len(parts) != 2 {
			return nil, newParseError(d, attr, syntaxError(l))
		}
		uevent[parts[0]] = parts[1]
	}
	return uevent, nil
}

func setAttributeOf(d Device, attr, data string) error {
	path := filepath.Join(d.Path(), d.String(), attr)
	err := ioutil.WriteFile(path, []byte(data), 0)
	if err != nil {
		return newAttrOpError(d, attr, data, "set", err)
	}
	return nil
}
