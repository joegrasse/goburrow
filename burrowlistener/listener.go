package burrowlistener

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/joegrasse/goburrow/common"
	ep "github.com/joegrasse/goburrow/endpoint"
	"golang.org/x/crypto/ssh"
)

// BurrowListener represents a network listener that can be local or remote.
type BurrowListener struct {
	// Endpoint is the endpoint of the BurrowListener. For dynamic port allocation,
	// you can do Endpoint.GetPort() to get the port used.
	Endpoint ep.Endpoint

	net.Listener

	serverConn *ssh.Client
	logger     *log.Logger
	proxy      ep.Endpoint
	sshServer  ep.Endpoint
}

type option func(*BurrowListener) error

// Logger sets the logger option for the BurrowListener
func Logger(logger *log.Logger) option {
	return func(bl *BurrowListener) error {
		bl.logger = logger

		return nil
	}
}

// Proxy sets the proxy option for the BurrowListener
func Proxy(proxy ep.Endpoint) option {
	return func(bl *BurrowListener) error {
		if proxy.IsSet() {
			bl.proxy = proxy
		}

		return nil
	}
}

// SSHServer sets the ssh server option for the BurrowListener
func SSHServer(sshServer ep.Endpoint) option {
	return func(bl *BurrowListener) error {
		if !sshServer.IsSet() {
			return nil
		}

		if sshServer.GetSSHConfig() == nil {
			return errors.New("ssh config not set on endpoint")
		}

		bl.sshServer = sshServer

		return nil
	}
}

// ListenBurrow listens on the endpoint, dialing through the sshServer and proxy if provided.
func ListenBurrow(endpoint ep.Endpoint, options ...option) (*BurrowListener, error) {
	var serverConn *ssh.Client
	var listener net.Listener
	var remoteEndpoint string
	var err error

	bl := &BurrowListener{}

	if err := bl.setOption(options...); err != nil {
		return nil, err
	}

	// check if we need a remote listener
	if bl.sshServer.IsSet() {
		// Dial the server
		serverConn, err = common.DialServer(bl.sshServer,
			common.Logger(bl.logger),
			common.Proxy(bl.proxy))

		if err != nil {
			return nil, err
		}

		// check remote address
		if endpoint.GetAddress() == "" {
			remoteEndpoint = fmt.Sprintf("%s:%d", "0.0.0.0", endpoint.GetPort())
		} else {
			remoteEndpoint = endpoint.String()
		}

		// Dial the remote server through ssh conn
		listener, err = serverConn.Listen("tcp", remoteEndpoint)

		if err != nil {
			bl.log("Closing Server Connection")
			serverConn.Close()

			return nil, fmt.Errorf("failed to listen on remote endpoint: %s", err)
		}

		bl.serverConn = serverConn
		bl.Listener = listener

		bl.Endpoint = endpoint
	} else {
		listener, err = net.Listen("tcp", endpoint.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create local listener: %s", err)
		}

		bl.Listener = listener

		bl.Endpoint = endpoint
	}

	// if port is automatically chosen, get it
	if bl.Endpoint.GetPort() == 0 {
		_, port, err := net.SplitHostPort(listener.Addr().String())
		if err == nil {
			p, err := strconv.Atoi(port)
			if err == nil {
				bl.Endpoint.SetPort(p)
				bl.log("Port chosen automatically: %d", p)
			}
		}
	}

	bl.log("Listening on: %s", bl.Endpoint)

	return bl, err
}

// setOption sets the options specified.
func (bl *BurrowListener) setOption(options ...option) error {
	for _, opt := range options {
		if err := opt(bl); err != nil {
			return err
		}
	}

	return nil
}

// Close stops listening on the Endpoint. For remote listeners,
// already accepted connections are not closed if closeSSH is
// false. For local listeners, already accepted connections are
// not closed either way.
func (bl *BurrowListener) Close(closeSSH bool) error {
	var err error

	if bl.Listener != nil {
		err = bl.Listener.Close()
	}

	if bl.serverConn != nil && closeSSH {
		err = bl.serverConn.Close()
	}

	bl.Listener = nil

	return err
}

// log implements logging for the listener
func (bl *BurrowListener) log(format string, args ...interface{}) {
	if bl.logger != nil {
		bl.logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}
