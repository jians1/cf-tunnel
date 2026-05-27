package connection

import (
	"sync"
)

const (
	MetricsNamespace = "cloudflared"
	TunnelSubsystem  = "tunnel"
	muxerSubsystem   = "muxer"
	configSubsystem  = "config"
)

type localConfigMetrics struct {
	pushes       noopMetric
	pushesErrors noopMetric
}

type tunnelMetrics struct {
	serverLocations noopMetricVec
	// locationLock is a mutex for oldServerLocations
	locationLock sync.Mutex
	// oldServerLocations stores the last server the tunnel was connected to
	oldServerLocations map[string]string

	regSuccess noopMetricVec
	regFail    noopMetricVec
	rpcFail    noopMetricVec

	tunnelsHA           tunnelsForHA
	userHostnamesCounts noopMetricVec

	localConfigMetrics *localConfigMetrics
}

type noopMetric struct{}

func (noopMetric) Inc() {}
func (noopMetric) Dec() {}

type noopMetricVec struct{}

func (noopMetricVec) WithLabelValues(labels ...string) noopMetric {
	return noopMetric{}
}

func newLocalConfigMetrics() *localConfigMetrics {
	return &localConfigMetrics{}
}

// Metrics that can be collected without asking the edge
func initTunnelMetrics() *tunnelMetrics {
	return &tunnelMetrics{
		oldServerLocations: make(map[string]string),
		tunnelsHA:          newTunnelsForHA(),
		localConfigMetrics: newLocalConfigMetrics(),
	}
}

func (t *tunnelMetrics) registerServerLocation(connectionID, loc string) {
	t.locationLock.Lock()
	defer t.locationLock.Unlock()
	if oldLoc, ok := t.oldServerLocations[connectionID]; ok && oldLoc == loc {
		return
	} else if ok {
		t.serverLocations.WithLabelValues(connectionID, oldLoc).Dec()
	}
	t.serverLocations.WithLabelValues(connectionID, loc).Inc()
	t.oldServerLocations[connectionID] = loc
}

var tunnelMetricsInternal struct {
	sync.Once
	metrics *tunnelMetrics
}

func newTunnelMetrics() *tunnelMetrics {
	tunnelMetricsInternal.Do(func() {
		tunnelMetricsInternal.metrics = initTunnelMetrics()
	})
	return tunnelMetricsInternal.metrics
}
