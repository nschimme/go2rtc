package streams

import "time"

// CFG is the package-level configuration for streams features.
var CFG struct {
	Backchannel struct {
		InactivityTimeout time.Duration `yaml:"inactivity_timeout"`
	} `yaml:"backchannel"`
}

// DefaultBackchannelInactivityTimeout is used if no value is configured.
// Set to 0 to disable the inactivity monitor by default if not specified in YAML.
// Or, InactiveReceiver can have its own internal default if this is 0.
// Let's make it 0, meaning "disabled by default unless configured".
const DefaultBackchannelInactivityTimeout = 0 * time.Second

func GetBackchannelInactivityTimeout() time.Duration {
	if CFG.Backchannel.InactivityTimeout > 0 {
		return CFG.Backchannel.InactivityTimeout
	}
	// If we want a non-zero default even if not configured, set it here.
	// But for explicit opt-in, 0 from YAML or unconfigured means disabled.
	// The InactiveReceiver itself has a DefaultInactiveReceiverTimeout if 0 is passed to it,
	// so this function can return 0 if we want "disabled by config".
	// Let's make it so that if config is 0, it's disabled.
	// If config is not set, it's also 0 (default for time.Duration).
	return CFG.Backchannel.InactivityTimeout
}
