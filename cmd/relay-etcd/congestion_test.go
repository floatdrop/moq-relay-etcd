package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/floatdrop/moq-go/pkg/relay/relaynet"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlogwriter"
)

// stubRTT is a minimal [quic.RTTStatsProvider]. Implementing it here, outside
// quic-go, is itself part of what these tests check: the fork's congestion
// hook must be usable without reaching into quic-go's internal packages.
type stubRTT struct{ rtt time.Duration }

func (s stubRTT) MinRTT() time.Duration      { return s.rtt }
func (s stubRTT) LatestRTT() time.Duration   { return s.rtt }
func (s stubRTT) SmoothedRTT() time.Duration { return s.rtt }

func TestCongestionOptionSetsController(t *testing.T) {
	for _, name := range congestionNames() {
		t.Run(name, func(t *testing.T) {
			opt, err := congestionOption(name)
			if err != nil {
				t.Fatalf("congestionOption(%q): %v", name, err)
			}
			var cfg quic.Config
			opt(&cfg)
			if cfg.Congestion == nil {
				t.Fatal("option did not set Config.Congestion")
			}
			cc := cfg.Congestion(stubRTT{rtt: 50 * time.Millisecond}, 1200, nil)
			if cc == nil {
				t.Fatal("factory returned a nil controller")
			}
			if cc.GetCongestionWindow() <= 0 {
				t.Fatalf("controller starts with a non-positive window: %d", cc.GetCongestionWindow())
			}
		})
	}
}

// TestCongestionOptionPicksDistinctControllers guards against every name
// silently resolving to the same controller, which would make an A/B between
// them meaningless. BBR leaves Startup on its own estimators and so ignores the
// MaybeExitSlowStart hint that Reno and CUBIC act on.
func TestCongestionOptionPicksDistinctControllers(t *testing.T) {
	newController := func(t *testing.T, name string) quic.CongestionController {
		t.Helper()
		opt, err := congestionOption(name)
		if err != nil {
			t.Fatalf("congestionOption(%q): %v", name, err)
		}
		var cfg quic.Config
		opt(&cfg)
		return cfg.Congestion(stubRTT{rtt: 50 * time.Millisecond}, 1200, nil)
	}

	bbr := newController(t, "bbr")
	bbr.MaybeExitSlowStart()
	if !bbr.InSlowStart() {
		t.Error("BBR should ignore the MaybeExitSlowStart hint")
	}

	// Reno and CUBIC both act on it, given a window above the slow start
	// threshold; here it is enough that they are not the BBR sender.
	for _, name := range []string{"reno", "cubic"} {
		if cc := newController(t, name); cc == bbr {
			t.Errorf("%s returned the same controller instance as bbr", name)
		}
	}
}

func TestCongestionOptionRejectsUnknown(t *testing.T) {
	if _, err := congestionOption("nope"); err == nil {
		t.Fatal("expected an error for an unknown controller name")
	}
}

// TestCongestionOptionReachesQUIC is the end-to-end check that matters for this
// repo: that the controller a flag selects is actually the one quic-go builds
// for a real connection, through relaynet's option plumbing and the forked
// quic.Config.Congestion field. If the fork's replace directive were dropped, or
// relaynet stopped forwarding the option, this is what would catch it.
func TestCongestionOptionReachesQUIC(t *testing.T) {
	var serverBuilt, clientBuilt atomic.Int64
	counting := func(n *atomic.Int64) relaynet.Option {
		return relaynet.WithQUICConfig(func(cfg *quic.Config) {
			cfg.Congestion = func(rtt quic.RTTStatsProvider, size quic.ByteCount, q qlogwriter.Recorder) quic.CongestionController {
				n.Add(1)
				return quic.NewReno(rtt, size, q)
			}
		})
	}

	tlsCfg, err := relaynet.TLSConfig("", "", relaynet.MOQTQUICALPNs)
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	ln, err := relaynet.Listen("127.0.0.1:0", "/moq", tlsCfg, slog.New(slog.DiscardHandler), counting(&serverBuilt))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	accepted := make(chan error, 1)
	go func() {
		conn, err := ln.Accept(ctx)
		if conn != nil {
			defer conn.CloseWithError(0, "")
		}
		accepted <- err
	}()

	clientTLS := relaynet.InsecureClientTLSConfig(relaynet.MOQTQUICALPNs)
	conn, err := relaynet.DialQUIC(ctx, ln.Addr().String(), clientTLS, counting(&clientBuilt))
	if err != nil {
		t.Fatalf("DialQUIC: %v", err)
	}
	defer conn.CloseWithError(0, "")

	if err := <-accepted; err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := clientBuilt.Load(); got == 0 {
		t.Error("the dial leg's congestion controller was never built")
	}
	if got := serverBuilt.Load(); got == 0 {
		t.Error("the listen leg's congestion controller was never built")
	}
}
