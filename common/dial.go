package common

import (
	"fmt"
	"log"
	"net"

	ep "github.com/joegrasse/goburrow/endpoint"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

type connection struct {
	connNumber int64
	logger     *log.Logger
	proxy      ep.Endpoint
}

// setOption sets the options specified.
func (c *connection) setOption(options ...option) error {
	for _, opt := range options {
		if err := opt(c); err != nil {
			return err
		}
	}

	return nil
}

// log implements logging for the conn.
func (c *connection) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	msg = fmt.Sprintf("[%d]: %s", c.connNumber, msg)

	if c.logger != nil {
		c.logger.Print(msg)
	} else {
		log.Print(msg)
	}
}

// proxyConn connects to remote endpoint through a proxy
func (c *connection) proxyConn(remote ep.Endpoint) (net.Conn, error) {
	c.log("Connecting to Proxy: %s", c.proxy)
	dialer, err := proxy.SOCKS5("tcp", c.proxy.String(), nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	c.log("Connected to Proxy: %s", c.proxy)

	c.log("Dialing endpoint through proxy: %s", remote)
	conn, err := dialer.Dial("tcp", remote.String())
	if err != nil {
		return nil, fmt.Errorf("proxy dial error: %s", err)
	}

	c.log("Connected to endpoint: %s", remote)

	return conn, nil
}

type option func(*connection) error

// ConnNumber sets the connection number option for the BurrowConn
func ConnNumber(number int64) option {
	return func(c *connection) error {
		c.connNumber = number

		return nil
	}
}

// Logger sets the logger option for the connection
func Logger(logger *log.Logger) option {
	return func(c *connection) error {
		c.logger = logger

		return nil
	}
}

// Proxy sets the proxy option for the connection
func Proxy(proxy ep.Endpoint) option {
	return func(c *connection) error {
		if proxy.IsSet() {
			c.proxy = proxy
		}

		return nil
	}
}

// DialServer makes an ssh connects to a remote server, using a proxy if necessary.
func DialServer(server ep.Endpoint, options ...option) (*ssh.Client, error) {
	var conn net.Conn
	var serverConn *ssh.Client
	var err error

	c := &connection{}

	if err := c.setOption(options...); err != nil {
		return nil, err
	}

	if c.proxy.IsSet() {
		conn, err = c.proxyConn(server)

		if err != nil {
			return nil, err
		}

		c.log("Creating SSH connection to server: %s", server)

		// create a new ssh client
		c, chans, reqs, err := ssh.NewClientConn(conn, server.String(), server.GetSSHConfig())
		if err != nil {
			return nil, fmt.Errorf("ssh connection error: %s", err)
		}

		serverConn = ssh.NewClient(c, chans, reqs)
	} else {
		c.log("Creating SSH connection to server: %s", server)
		serverConn, err = ssh.Dial("tcp", server.String(), server.GetSSHConfig())
	}

	if err != nil {
		return nil, fmt.Errorf("server dial error: %s", err)
	}

	c.log("SSH connection created: %s", server)

	return serverConn, nil
}

// DialRemote connects to a remote server, using a proxy if necessary.
func DialRemote(remote ep.Endpoint, options ...option) (net.Conn, error) {
	var remoteConn net.Conn
	var err error

	c := &connection{}

	if err := c.setOption(options...); err != nil {
		return nil, err
	}

	if c.proxy.IsSet() {
		remoteConn, err = c.proxyConn(remote)
	} else {
		c.log("Dialing remote: %s", remote)
		remoteConn, err = net.Dial("tcp", remote.String())
	}

	if err != nil {
		return nil, err
	}

	c.log("Remote connection created: %s", remote)

	return remoteConn, err
}
