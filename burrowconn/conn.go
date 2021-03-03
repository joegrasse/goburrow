package burrowconn

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/joegrasse/goburrow/common"
	ep "github.com/joegrasse/goburrow/endpoint"
	"golang.org/x/crypto/ssh"
)

// BurrowConn represents a network connection through a proxy
// and/or ssh server if necessary.
type BurrowConn struct {
	net.Conn
	connNumber int64
	serverConn *ssh.Client
	logger     *log.Logger
	proxy      ep.Endpoint
	sshServer  ep.Endpoint
}

type option func(*BurrowConn) error

// ConnNumber sets the connection number option for the BurrowConn
func ConnNumber(number int64) option {
	return func(bc *BurrowConn) error {
		bc.connNumber = number

		return nil
	}
}

// Logger sets the logger option for the BurrowConn
func Logger(logger *log.Logger) option {
	return func(bc *BurrowConn) error {
		bc.logger = logger

		return nil
	}
}

// Proxy sets the proxy option for the BurrowConn
func Proxy(proxy ep.Endpoint) option {
	return func(bc *BurrowConn) error {
		if proxy.IsSet() {
			bc.proxy = proxy
		}

		return nil
	}
}

// SSHServer sets the ssh server option for the BurrowConn
func SSHServer(sshServer ep.Endpoint) option {
	return func(bc *BurrowConn) error {
		if !sshServer.IsSet() {
			return nil
		}

		if sshServer.GetSSHConfig() == nil {
			return errors.New("ssh config not set on endpoint")
		}

		bc.sshServer = sshServer

		return nil
	}
}

// DialBurrow connects to the endpoint using a ssh server and/or proxy, if provided.
func DialBurrow(endpoint ep.Endpoint, options ...option) (*BurrowConn, error) {
	var serverConn *ssh.Client
	var remoteEndpoint string
	var conn net.Conn
	var err error

	bc := &BurrowConn{}

	if err := bc.setOption(options...); err != nil {
		return nil, err
	}

	// check if going through remote server for burrow
	if bc.sshServer.IsSet() {
		// Dial the server
		serverConn, err = common.DialServer(bc.sshServer,
			common.Logger(bc.logger),
			common.Proxy(bc.proxy),
			common.ConnNumber(bc.connNumber))

		if err != nil {
			return nil, err
		}

		bc.log("Dialing: %s", endpoint)

		// check remote address
		if endpoint.GetAddress() == "" {
			remoteEndpoint = fmt.Sprintf("%s:%d", "0.0.0.0", endpoint.GetPort())
		} else {
			remoteEndpoint = endpoint.String()
		}

		// Dial the remote server through ssh conn
		conn, err = serverConn.Dial("tcp", remoteEndpoint)

		if err != nil {
			bc.log("Closing Server Connection")
			serverConn.Close()

			return nil, fmt.Errorf("Failed to connect to remote endpoint: %s", err)
		}

		bc.log("Connection created: %s", endpoint)

		bc.serverConn = serverConn
		bc.Conn = conn

	} else {
		// Dial remote server
		conn, err = common.DialRemote(endpoint,
			common.Logger(bc.logger),
			common.Proxy(bc.proxy),
			common.ConnNumber(bc.connNumber))

		if err != nil {
			return nil, fmt.Errorf("Failed to connect to remote endpoint: %s", err)
		}

		bc.Conn = conn
	}

	return bc, err
}

// setOption sets the options specified.
func (bc *BurrowConn) setOption(options ...option) error {
	for _, opt := range options {
		if err := opt(bc); err != nil {
			return err
		}
	}

	return nil
}

// Close closes the connection.
func (bc *BurrowConn) Close() error {
	err := bc.Conn.Close()
	if bc.serverConn != nil {
		err = bc.serverConn.Close()
	}

	return err
}

// log implements logging for the conn.
func (bc *BurrowConn) log(format string, args ...interface{}) {

	msg := fmt.Sprintf(format, args...)
	msg = fmt.Sprintf("[%d]: %s", bc.connNumber, msg)

	if bc.logger != nil {
		bc.logger.Print(msg)
	} else {
		log.Print(msg)
	}
}
