package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/floatdrop/moq-go/pkg/relay/relaynet"
	"github.com/quic-go/quic-go"
)

// congestionControllers are the controllers this relay can be told to use.
//
// The relay's two legs have very different paths under them, which is why they
// are selected separately (see the -congestion and -upstream-congestion flags).
// Downstream connections run to clients over whatever last mile they have --
// wifi, cellular, a tunnel -- where packets are lost and reordered for reasons
// that have nothing to do with congestion. Reno reads every one of those as
// congestion and halves its window, which on such a path pins the window near
// its floor; that stalls the relay's per-subscriber send queues and turns into
// dropped objects and TOO_FAR_BEHIND terminations rather than merely slower
// transfers. Cross-relay upstream legs usually run over a clean datacenter
// path, where the choice matters much less.
//
// This map is the whole reason the fork exists: quic.Config.Congestion is not a
// field upstream quic-go has. See the module's replace directive.
var congestionControllers = map[string]quic.CongestionControllerFactory{
	"bbr":   quic.NewBBRv3,
	"reno":  quic.NewReno,
	"cubic": quic.NewCubic,
}

// congestionNames lists the valid -congestion values, for flag help and errors.
func congestionNames() []string {
	return slices.Sorted(maps.Keys(congestionControllers))
}

// congestionOption turns a controller name into the relaynet [relaynet.Option]
// that installs it, leaving every other default relaynet sets in place.
func congestionOption(name string) (relaynet.Option, error) {
	factory, ok := congestionControllers[name]
	if !ok {
		return nil, fmt.Errorf("unknown congestion controller %q (want one of %v)", name, congestionNames())
	}
	return relaynet.WithQUICConfig(func(cfg *quic.Config) {
		cfg.Congestion = factory
	}), nil
}
