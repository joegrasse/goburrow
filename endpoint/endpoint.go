package endpoint

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Endpoint represents a network endpoint
type Endpoint struct {
	address   string
	port      int
	set       bool
	sshConfig *ssh.ClientConfig
}

// NewEndpoint returns a new Endpoint. Value can be one of
// host:port, :port, or host
func NewEndpoint(value string) (Endpoint, error) {
	e := Endpoint{}

	err := e.Set(value)

	return e, err
}

// Network returns the name of the network
func (e *Endpoint) Network() string {
	return "tcp"
}

// String returns the string form of the endpoint
func (e Endpoint) String() string {
	if !e.set {
		return ""
	}

	return fmt.Sprintf("%s:%d", e.address, e.port)
}

// Set sets the endpoint values
func (e *Endpoint) Set(value string) error {
	if value == "" {
		return errors.New("endpoint can not be blank")
	}

	if e == nil {
		return errors.New("endpoint has not been initialized")
	}

	// split for port
	endpointParts := strings.SplitN(value, ":", 2)
	if len(endpointParts) > 1 {
		// check for blank port
		if endpointParts[1] == "" {
			e.port = 0
		} else { // not blank, try and parse as number
			i, err := strconv.Atoi(endpointParts[1])
			if err != nil {
				return fmt.Errorf("error parsing port: \"%s\"", endpointParts[1])
			}
			e.port = i
		}
	}

	e.address = endpointParts[0]

	// mark that the value has been set
	e.set = true

	return nil
}

// Type returns the value type
func (e *Endpoint) Type() string {
	return "endpoint"
}

// IsSet returns whether the endpoint has been set
func (e *Endpoint) IsSet() bool {
	if e == nil {
		return false
	}

	if !e.set {
		return false
	}

	return true
}

// SetAddress sets the endpoint's address
func (e *Endpoint) SetAddress(address string) {
	e.address = address
}

// SetPort sets the endpoint's port
func (e *Endpoint) SetPort(port int) {
	e.port = port
}

// SetSSHConfig sets the endpoint's ssh config
func (e *Endpoint) SetSSHConfig(c *ssh.ClientConfig) {
	e.sshConfig = c
}

// GetAddress returns the endpoint's address
func (e *Endpoint) GetAddress() string {
	if !e.set {
		return ""
	}
	return e.address
}

// GetPort returns the endpoint's port
func (e *Endpoint) GetPort() int {
	if !e.set {
		return 0
	}
	return e.port
}

// GetSSHConfig returns the endpoint's ssh config
func (e *Endpoint) GetSSHConfig() *ssh.ClientConfig {
	return e.sshConfig
}
