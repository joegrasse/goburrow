package goburrow

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bc "github.com/joegrasse/goburrow/burrowconn"
	bl "github.com/joegrasse/goburrow/burrowlistener"
	"github.com/joegrasse/goburrow/endpoint"
)

// Burrow represents the burrowed network connections
type Burrow struct {
	Local    endpoint.Endpoint
	Remote   endpoint.Endpoint
	Server   endpoint.Endpoint
	Proxy    endpoint.Endpoint
	InfoLog  *log.Logger
	DebugLog *log.Logger
	Reverse  bool
	Tries    int

	connections  int64
	currentConns int64
	state        int32
	listener     *bl.BurrowListener
	doneChan     chan struct{}
	startedChan  chan struct{}
	activeConn   map[chan struct{}]net.Conn
	mux          sync.Mutex
}

type option func(*Burrow) error

// DebugLog sets the debug log option for the Burrow.
func DebugLog(logger *log.Logger) option {
	return func(b *Burrow) error {
		b.DebugLog = logger

		return nil
	}
}

// InfoLog sets the info log option for the Burrow.
func InfoLog(logger *log.Logger) option {
	return func(b *Burrow) error {
		b.InfoLog = logger

		return nil
	}
}

// Proxy sets the proxy option for the Burrow
func Proxy(proxy string) option {
	return func(b *Burrow) error {
		var err error

		if proxy == "" {
			return nil
		}

		if b.Proxy, err = endpoint.NewEndpoint(proxy); err != nil {
			return fmt.Errorf("Proxy Endpoint: %s", err)
		}

		return nil
	}
}

// Tries sets the number of connection attempts for the Burrow
func Tries(tries int) option {
	return func(b *Burrow) error {
		b.Tries = tries

		return nil
	}
}

// Reverse sets the reverse option for the Burrow
func Reverse(reverse bool) option {
	return func(b *Burrow) error {
		b.Reverse = reverse

		return nil
	}
}

// Server sets the server option for the Burrow
func Server(server string) option {
	return func(b *Burrow) error {
		var err error

		if server == "" {
			return nil
		}

		if b.Server, err = endpoint.NewEndpoint(server); err != nil {
			return fmt.Errorf("Server Endpoint: %s", err)
		}

		return nil
	}
}

// NewBurrow returns a new Burrow or error
func NewBurrow(local, remote string, options ...option) (*Burrow, error) {
	var err error
	b := &Burrow{}

	if b.Local, err = endpoint.NewEndpoint(local); err != nil {
		return nil, fmt.Errorf("Local Endpoint: %s", err)
	}

	if b.Remote, err = endpoint.NewEndpoint(remote); err != nil {
		return nil, fmt.Errorf("Remote Endpoint: %s", err)
	}

	if err := b.setOption(options...); err != nil {
		return nil, err
	}

	if err = b.valid(); err != nil {
		return nil, err
	}

	return b, nil
}

// setOption sets the options specified.
func (b *Burrow) setOption(options ...option) error {
	for _, opt := range options {
		if err := opt(b); err != nil {
			return err
		}
	}

	return nil
}

func (b *Burrow) infoLog(format string, args ...interface{}) {
	logEntry := fmt.Sprintf(format, args...)
	logEntries := strings.Split(logEntry, "\n")

	for _, le := range logEntries {
		if b.InfoLog != nil {
			b.InfoLog.Print(le)
		} else {
			log.Print(le)
		}
	}
}

func (b *Burrow) infoLogConn(conn int64, format string, args ...interface{}) {
	logEntry := fmt.Sprintf(format, args...)
	logEntries := strings.Split(logEntry, "\n")

	for _, le := range logEntries {
		if b.InfoLog != nil {
			b.InfoLog.Printf("[%d]: %s", conn, le)
		} else {
			log.Printf("[%d]: %s", conn, le)
		}
	}
}

func (b *Burrow) debugLog(format string, args ...interface{}) {
	logEntry := fmt.Sprintf(format, args...)
	logEntries := strings.Split(logEntry, "\n")

	for _, le := range logEntries {
		if b.DebugLog != nil {
			b.DebugLog.Print(le)
		} else {
			log.Print(le)
		}
	}
}

func (b *Burrow) debugLogConn(conn int64, format string, args ...interface{}) {
	logEntry := fmt.Sprintf(format, args...)
	logEntries := strings.Split(logEntry, "\n")

	for _, le := range logEntries {
		if b.DebugLog != nil {
			b.DebugLog.Printf("[%d]: %s", conn, le)
		} else {
			log.Printf("[%d]: %s", conn, le)
		}
	}
}

func connectString(b *Burrow) string {
	connection := fmt.Sprintf("Started - Local: %s Remote: %s", b.Local, b.Remote)

	// check for server address
	if b.Server.IsSet() {
		connection = fmt.Sprintf("%s Server: %s", connection, b.Server)
	}

	// check for proxy address
	if b.Proxy.IsSet() {
		connection = fmt.Sprintf("%s Proxy: %s", connection, b.Proxy)
	}

	return connection
}

func (b *Burrow) setState(state BurrowState) {
	atomic.StoreInt32(&b.state, int32(state))
}

// GetState returns the current state of the burrow
func (b *Burrow) GetState() BurrowState {
	atomic.LoadInt32(&b.state)

	// if started and has connections, return active
	if b.state == int32(StateStarted) && b.Connections() > 0 {
		return StateActive
	}

	return BurrowState(b.state)
}

// valid checks if the burrow settings are valid
func (b *Burrow) valid() error {

	// check local endpoint
	if !b.Local.IsSet() {
		return fmt.Errorf("Local Endpoint is required")
	}

	if b.Reverse && b.Local.GetPort() == 0 {
		return fmt.Errorf("Local Endpoint port can not be 0 for a reverse burrow")
	}

	// check remote endpoint
	if !b.Remote.IsSet() {
		return fmt.Errorf("Remote Endpoint is required")
	}

	if !b.Reverse && b.Remote.GetPort() == 0 {
		return fmt.Errorf("Remote Endpoint port can not be 0")
	}

	// check server endpoint
	if b.Server.IsSet() && b.Server.GetPort() == 0 {
		b.Server.SetPort(22)
	}

	if b.Reverse && !b.Server.IsSet() {
		return fmt.Errorf("Server Endpoint is required for reverse burrow")
	}

	// check proxy endpoint
	if b.Proxy.IsSet() && b.Proxy.GetPort() == 0 {
		return fmt.Errorf("Proxy Endpoint port can not be 0")
	}

	return nil
}

// Start the burrow
func (b *Burrow) Start() error {
	var err error

	if err = b.valid(); err != nil {
		b.setState(StateStopped)
		return err
	}

	// Check that the burrow hasn't already been started
	currentState := b.GetState()
	if currentState != StateNew && currentState != StateStopped {
		return fmt.Errorf("burrow already started")
	} else if currentState == StateStopped {
		// reusing burrow, reset done channel
		b.reset()
	}

	b.setState(StateStarting)

	// start listener
	if b.Reverse {
		b.listener, err = bl.ListenBurrow(b.Remote,
			bl.Logger(b.DebugLog),
			bl.SSHServer(b.Server),
			bl.Proxy(b.Proxy))
	} else {
		b.listener, err = bl.ListenBurrow(b.Local,
			bl.Logger(b.DebugLog),
			bl.Proxy(b.Proxy))
	}

	if err != nil {
		b.closeStartedChan()
		b.setState(StateStopped)
		return err
	}

	defer b.listener.Close(true)

	// update port if using a dynamic port
	if b.Reverse && b.Remote.GetPort() == 0 {
		b.Remote.SetPort(b.listener.Endpoint.GetPort())
	} else if !b.Reverse && b.Local.GetPort() == 0 {
		b.Local.SetPort(b.listener.Endpoint.GetPort())
	}

	b.markStarted()

	b.infoLog(connectString(b))

	for {
		conn, err := b.listener.Accept()

		if err != nil {
			state := b.GetState()
			if state == StateStopped || state == StateShuttingDown {
				<-b.getDoneChan()
				b.infoLog("Burrow Stopped")
				return nil
			}

			b.setState(StateStopped)
			b.infoLog("Burrow Stopped")
			return err
		}

		go func() {
			// create done channel for new burrow
			doneCh := make(chan struct{})

			// Get connection port number
			connNumber := atomic.AddInt64(&b.connections, 1)

			b.infoLogConn(connNumber, "Client accepted: %s", b.listener.Endpoint)

			b.setActiveChan(doneCh, conn, true, connNumber)

			// set up retry loop
			var (
				errOrigin      origin
				connectSuccess bool
				tries          int
				checkDone      bool
			)
		Retry:
			for !b.stoppedStarted() {
				checkDone = false

				if tries != 0 {
					b.debugLogConn(connNumber, "Connection Attempt [%d]", tries+1)
				}

				connectSuccess, errOrigin, err = b.burrow(conn, connNumber, doneCh)
				tries++

				// if local error, stop retries
				if err != nil && errOrigin == originLocal {
					break
				}

				// reset tries on successful connect
				if connectSuccess {
					tries = 0
				}

				// if no error break out of loop
				if err == nil {
					break
				}

				// print connection attempt error
				if err != nil {
					b.debugLogConn(connNumber, "%s", err)
				}

				// check if we need to retry connection
				if tries >= b.Tries && b.Tries != 0 {
					checkDone = true
					break
				}

				//waitTime := 1 * time.Second
				// increase the time between retries
				waitTime := time.Duration(math.Round(math.Log2(float64(tries+1))*1000)) * time.Millisecond
				timeChan := time.NewTimer(waitTime).C

				// check for timer or done
				select {
				case <-timeChan:
					checkDone = true
					continue
				case <-doneCh:
					break Retry
				}
			}

			// check if we need to get done signal
			if checkDone && b.stoppedStarted() {
				<-doneCh
			}

			b.debugLogConn(connNumber, "Closing Listener Connection")
			conn.Close()

			// close done channel
			b.setActiveChan(doneCh, conn, false, connNumber)
			// atomic.AddInt64(&b.currentConns, -1)

			if err != nil {
				b.infoLogConn(connNumber, "%s", err)

				// only stop burrow on remote error
				if errOrigin == originRemote {
					b.Stop()
				}
			}
		}()
	}
}

func (b *Burrow) reset() {
	b.mux.Lock()
	defer b.mux.Unlock()

	b.startedChan = nil
	b.doneChan = nil
	b.activeConn = nil
}

func (b *Burrow) markStarted() {
	b.setState(StateStarted)
	b.closeStartedChan()
}

// WaitStarted returns as soon as the burrow is no longer
// in StateNew
func (b *Burrow) WaitStarted() {
	ch := b.getStartedChan()

	select {
	case <-ch:
		// started
	}
}

func (b *Burrow) getStartedChan() <-chan struct{} {
	b.mux.Lock()
	defer b.mux.Unlock()
	return b.getStartedChanLocked()
}

func (b *Burrow) getStartedChanLocked() chan struct{} {
	if b.startedChan == nil {
		b.startedChan = make(chan struct{})
	}
	return b.startedChan
}

func (b *Burrow) closeStartedChan() {
	b.mux.Lock()
	defer b.mux.Unlock()

	ch := b.getStartedChanLocked()
	select {
	case <-ch:
		// Closed
	default:
		close(ch)
	}
}

func (b *Burrow) getDoneChan() <-chan struct{} {
	b.mux.Lock()
	defer b.mux.Unlock()
	return b.getDoneChanLocked()
}

func (b *Burrow) getDoneChanLocked() chan struct{} {
	if b.doneChan == nil {
		b.doneChan = make(chan struct{})
	}
	return b.doneChan
}

func (b *Burrow) closeDoneChan() {
	b.mux.Lock()
	defer b.mux.Unlock()

	ch := b.getDoneChanLocked()
	select {
	case <-ch:
		// Closed
	default:
		close(ch)
	}
}

func (b *Burrow) stoppedStarted() bool {
	currentState := b.GetState()
	if currentState == StateShuttingDown || currentState == StateStopped {
		return true
	}

	return false
}

// Stop immediately closes all active burrow
// listeners and any connections through it.
func (b *Burrow) Stop() error {
	var err error

	b.debugLog("Stopping Burrow")

	// check if we are already stopping
	if b.stoppedStarted() {
		return fmt.Errorf("Burrow is shutting down or has been stopped")
	}

	b.setState(StateShuttingDown)
	defer b.setState(StateStopped)

	if b.listener != nil {
		b.debugLog("Closing Burrow Listener")
		err = b.listener.Close(true)
	}

	b.signalClose()

	b.debugLog("Waiting for all connections to close")
	b.waitConnClosed()

	b.debugLog("All connections closed")
	b.closeDoneChan()

	return err
}

var shutdownPollInterval = 500 * time.Millisecond

// StopGraceful gracefully stops the burrow without
// interrupting any active connections. StopGraceful
// first closes the listener and then waits until
// there are no more active connections.
func (b *Burrow) StopGraceful() error {
	var err error

	b.debugLog("Stopping Burrow")

	// check if we are already stopping
	if b.stoppedStarted() {
		b.mux.Unlock()
		return fmt.Errorf("Burrow is shutting down or has been stopped")
	}

	b.setState(StateShuttingDown)
	defer b.setState(StateStopped)

	if b.listener != nil {
		b.debugLog("Closing Burrow Listener")
		err = b.listener.Close(true)
	}

	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()

	b.debugLog("Waiting for all connections to close")

	for {
		if b.Connections() == 0 {
			b.debugLog("All connections closed")
			b.closeDoneChan()
			return err
		}
		select {
		case <-ticker.C:
		}
	}
}

func (b *Burrow) setActiveChan(c chan struct{}, conn net.Conn, add bool, connNumber int64) {
	b.mux.Lock()
	defer b.mux.Unlock()
	if b.activeConn == nil {
		b.activeConn = make(map[chan struct{}]net.Conn)
	}

	if add {
		b.debugLogConn(connNumber, "Adding connection to active connections")
		b.activeConn[c] = conn
		atomic.AddInt64(&b.currentConns, 1)
	} else {
		b.debugLogConn(connNumber, "Removing connection from active connections")
		close(c)
		delete(b.activeConn, c)
		atomic.AddInt64(&b.currentConns, -1)
	}
}

func (b *Burrow) signalClose() {
	b.debugLog("Signaling active connections to close")

	b.mux.Lock()
	defer b.mux.Unlock()

	for c := range b.activeConn {
		b.debugLog("Sending signal")
		c <- struct{}{}
	}
	b.debugLog("Done signaling active connections to close")
}

func (b *Burrow) waitConnClosed() {
	b.mux.Lock()
	done := merge(b.activeConn)
	b.mux.Unlock()

	b.debugLog("Waiting for done on merged channel")
	select {
	case <-done:
		// done
	}
}

// merge merges multiple channels into one channel
func merge(cs map[chan struct{}]net.Conn) <-chan struct{} {
	var wg sync.WaitGroup
	out := make(chan struct{})

	// Start a wait goroutine for each channel in cs.  chanWait waits
	// until c is closed, then calls wg.Done.
	chanWait := func(c chan struct{}) {
		<-c
		wg.Done()
	}

	wg.Add(len(cs))
	for c := range cs {
		go chanWait(c)
	}

	// Start a goroutine to close out once all the chanWait
	// goroutines are done.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Connections returns the current number of connection through the burrow
func (b *Burrow) Connections() int64 {
	return atomic.LoadInt64(&b.currentConns)
}

type origin int

const (
	originNone origin = iota
	originLocal
	originRemote
)

var originName = map[origin]string{
	originNone:   "None",
	originLocal:  "Local",
	originRemote: "Remote",
}

func (t origin) String() string {
	return originName[t]
}

type copyConnResult struct {
	info      string
	err       error
	bytes     int64
	errOrigin origin
}

func (b *Burrow) burrow(listenerConn net.Conn, connNumber int64, done chan struct{}) (bool, origin, error) {
	var dialerConn *bc.BurrowConn
	var err error

	resultChannel := make(chan copyConnResult)

	if b.Reverse {
		dialerConn, err = bc.DialBurrow(b.Local,
			bc.Logger(b.DebugLog),
			bc.ConnNumber(connNumber))
	} else {
		dialerConn, err = bc.DialBurrow(b.Remote,
			bc.Logger(b.DebugLog),
			bc.SSHServer(b.Server),
			bc.Proxy(b.Proxy),
			bc.ConnNumber(connNumber))
	}

	if err != nil {
		return false, originRemote, fmt.Errorf("%s", err)
	}

	defer func() {
		b.debugLogConn(connNumber, "Closing Dialer Connection")
		dialerConn.Close()
	}()

	// start local <-> remote data transfer
	go func() {
		resultChannel <- copyConn(listenerConn, dialerConn)
	}()

	// check for errors or done
	select {
	case result := <-resultChannel:
		// check for error and bytes copied
		if result.err != nil && result.bytes > 0 {
			// had a successful connect
			return true, result.errOrigin, result.err
		} else if result.err != nil { // check for error
			// didn't have a successful connect
			return false, result.errOrigin, fmt.Errorf("Unable to communicate over dialed connection: %s", result.err)
		}

		// no error
		b.infoLogConn(connNumber, "%s", result.info)

		return true, originNone, nil
	case <-done:
		return true, originNone, nil
	}
}

// copyConn copies IO between local and remote conns
func copyConn(local, remote net.Conn) copyConnResult {
	resultChannel := make(chan copyConnResult)

	// copy remote -> local
	go func() {
		b, err := io.Copy(local, remote)
		if err != nil {
			resultChannel <- copyConnResult{err: fmt.Errorf("Error while communicating to local endpoint: %s", err), bytes: b, errOrigin: originRemote}
		} else {
			resultChannel <- copyConnResult{info: "Connection closed", bytes: b, errOrigin: originRemote}
		}
	}()

	// copy local -> remote
	go func() {
		b, err := io.Copy(remote, local)
		if err != nil {
			resultChannel <- copyConnResult{err: fmt.Errorf("Error while communicating to remote endpoint: %s", err), bytes: b, errOrigin: originLocal}
		} else {
			resultChannel <- copyConnResult{info: "Connection closed", bytes: b, errOrigin: originLocal}
		}
	}()

	// wait until done
	result := <-resultChannel

	return result
}
