package goburrow

// BurrowState represents the state of the burrow.
type BurrowState int

const (
	// StateNew represents a newly initialized burrow.
	StateNew BurrowState = iota

	// StateStarting represents a burrow that just starting up.
	StateStarting

	// StateStarted represents a burrow that has been started but
	// has no current connections.
	StateStarted

	// StateActive represents a burrow that has been started and
	// has current connections.
	StateActive

	// StateShuttingDown represents a burrow that is shutting down.
	StateShuttingDown

	// StateStopped represents a burrow that completely stopped.
	StateStopped
)

var stateName = map[BurrowState]string{
	StateNew:          "New",
	StateStarting:     "Starting",
	StateStarted:      "Started",
	StateActive:       "Active",
	StateShuttingDown: "Shutting Down",
	StateStopped:      "Stopped",
}

func (t BurrowState) String() string {
	return stateName[t]
}
