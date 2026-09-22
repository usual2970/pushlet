package pushlet

import "errors"

var (
	errInvalidRelayEnvelope = errors.New("pushlet: invalid relay envelope")
	errConnectorNotRunning  = errors.New("pushlet: distributed connector not running")
)
