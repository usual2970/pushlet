package pushlet

import "errors"

var (
	errInvalidRelayEnvelope   = errors.New("pushlet: invalid relay envelope")
	errConnectorNotRunning    = errors.New("pushlet: distributed connector not running")
	errBrokerNotRunning       = errors.New("pushlet: broker not running")
	errDistributedAlreadyOn   = errors.New("pushlet: distributed mode already enabled")
)
