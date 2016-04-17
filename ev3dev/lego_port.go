// Copyright ©2016 Dan Kortschak. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ev3dev

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
)

// LegoPortPath is the path to the ev3 lego-port file system.
const LegoPortPath = "/sys/class/lego-port"

// LegoPort represents a handle to a lego-port.
type LegoPort struct {
	mu sync.Mutex
	id int
}

// String satisfies the fmt.Stringer interface.
func (p *LegoPort) String() string { return fmt.Sprint(portPrefix, p.id) }

// LegoPortFor returns the LegoPort for the given ev3 port name.
func LegoPortFor(name string) (*LegoPort, error) {
	if strings.HasPrefix(name, "in") || strings.HasPrefix(name, "out") {
		for i := 0; i < 8; i++ {
			addr, err := (&LegoPort{id: i}).Address()
			if err != nil {
				return nil, err
			}
			if name == addr {
				return &LegoPort{id: i}, nil
			}
		}
	}
	return nil, fmt.Errorf("ev3dev: invalid port name: %q", name)
}

func (p *LegoPort) writeFile(path, data string) error {
	defer p.mu.Unlock()
	p.mu.Lock()
	return ioutil.WriteFile(path, []byte(data), 0)
}

// Address returns the ev3 port name for the LegoPort.
func (p *LegoPort) Address() (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(LegoPortPath+"/%s/"+address, p))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read port address: %v", err)
	}
	return string(chomp(b)), err
}

// Driver returns the driver name for the device registered to the LegoPort.
func (p *LegoPort) Driver() (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(LegoPortPath+"/%s/"+driverName, p))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read port driver name: %v", err)
	}
	return string(chomp(b)), err
}

// Modes returns the available modes for the LegoPort.
func (p *LegoPort) Modes() ([]string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(LegoPortPath+"/%s/"+modes, p))
	if err != nil {
		return nil, fmt.Errorf("ev3dev: failed to read port modes: %v", err)
	}
	return strings.Split(string(chomp(b)), " "), err
}

// Mode returns the currently selected mode of the LegoPort.
func (p *LegoPort) Mode() (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(LegoPortPath+"/%s/"+mode, p))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read port mode: %v", err)
	}
	return string(chomp(b)), err
}

// SetMode sets the mode of the LegoPort.
func (p *LegoPort) SetMode(mode string) error {
	err := p.writeFile(fmt.Sprintf(LegoPortPath+"/%s/"+mode, p), mode)
	if err != nil {
		return fmt.Errorf("ev3dev: failed to set port mode: %v", err)
	}
	return nil
}

// SetDevice sets the device of the LegoPort.
func (p *LegoPort) SetDevice(dev string) error {
	err := p.writeFile(fmt.Sprintf(LegoPortPath+"/%s/"+setDevice, p), dev)
	if err != nil {
		return fmt.Errorf("ev3dev: failed to set port device: %v", err)
	}
	return nil
}

// Status returns the current status of the LegoPort.
func (p *LegoPort) Status() (string, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf(LegoPortPath+"/%s/"+status, p))
	if err != nil {
		return "", fmt.Errorf("ev3dev: failed to read port status: %v", err)
	}
	return string(chomp(b)), err
}

// ConnectedTo returns a description of the device attached to p in the form
// {inX,outY}:DEVICE-NAME, where X is in {1-4} and Y is in {A-D}.
func ConnectedTo(p *LegoPort) (string, error) {
	if p.id < 0 {
		return "", fmt.Errorf("ev3dev: invalid lego port number: %d", p.id)
	}
	f, err := os.Open(fmt.Sprintf(LegoPortPath+"/%s", p))
	if err != nil {
		return "", err
	}
	defer f.Close()
	names, err := f.Readdirnames(0)
	if err != nil {
		return "", err
	}
	for _, n := range names {
		switch {
		case strings.HasPrefix(n, "in"):
			if len(n) >= 4 && n[3] == ':' && '1' <= n[2] && n[2] <= '4' {
				return n, nil
			}
		case strings.HasPrefix(n, "out"):
			if len(n) >= 5 && n[4] == ':' && 'A' <= n[3] && n[3] <= 'D' {
				return n, nil
			}
		}
	}
	return "", nil
}
