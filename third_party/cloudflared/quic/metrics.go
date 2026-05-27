package quic

import (
	"reflect"
	"strings"

	"github.com/quic-go/quic-go/logging"
	"github.com/rs/zerolog"
)

const (
	namespace                  = "quic"
	ConnectionIndexMetricLabel = "conn_index"
	frameTypeMetricLabel       = "frame_type"
	packetTypeMetricLabel      = "packet_type"
	reasonMetricLabel          = "reason"
)

var (
	clientMetrics = struct {
		totalConnections  noopMetric
		closedConnections noopMetric
		maxUDPPayloadSize noopMetricVec
		sentFrames        noopMetricVec
		sentBytes         noopMetricVec
		receivedFrames    noopMetricVec
		receivedBytes     noopMetricVec
		bufferedPackets   noopMetricVec
		droppedPackets    noopMetricVec
		lostPackets       noopMetricVec
		minRTT            noopMetricVec
		latestRTT         noopMetricVec
		smoothedRTT       noopMetricVec
		mtu               noopMetricVec
		congestionWindow  noopMetricVec
		congestionState   noopMetricVec
	}{}

	packetTooBigDropped noopMetric
)

type noopMetric struct{}

func (noopMetric) Inc()              {}
func (noopMetric) Add(value float64) {}
func (noopMetric) Set(value float64) {}

type noopMetricVec struct{}

func (noopMetricVec) WithLabelValues(labels ...string) noopMetric {
	return noopMetric{}
}

type clientCollector struct {
	index  string
	logger *zerolog.Logger
}

func newClientCollector(index string, logger *zerolog.Logger) *clientCollector {
	return &clientCollector{
		index:  index,
		logger: logger,
	}
}

func (cc *clientCollector) startedConnection() {
	clientMetrics.totalConnections.Inc()
}

func (cc *clientCollector) closedConnection(error) {
	clientMetrics.closedConnections.Inc()
}

func (cc *clientCollector) receivedTransportParameters(params *logging.TransportParameters) {
	clientMetrics.maxUDPPayloadSize.WithLabelValues(cc.index).Set(float64(params.MaxUDPPayloadSize))
	cc.logger.Debug().Msgf("Received transport parameters: MaxUDPPayloadSize=%d, MaxIdleTimeout=%v, MaxDatagramFrameSize=%d", params.MaxUDPPayloadSize, params.MaxIdleTimeout, params.MaxDatagramFrameSize)
}

func (cc *clientCollector) sentPackets(size logging.ByteCount, frames []logging.Frame) {
	cc.collectPackets(size, frames, clientMetrics.sentFrames, clientMetrics.sentBytes, sent)
}

func (cc *clientCollector) receivedPackets(size logging.ByteCount, frames []logging.Frame) {
	cc.collectPackets(size, frames, clientMetrics.receivedFrames, clientMetrics.receivedBytes, received)
}

func (cc *clientCollector) bufferedPackets(packetType logging.PacketType) {
	clientMetrics.bufferedPackets.WithLabelValues(cc.index, packetTypeString(packetType)).Inc()
}

func (cc *clientCollector) droppedPackets(packetType logging.PacketType, size logging.ByteCount, reason logging.PacketDropReason) {
	clientMetrics.droppedPackets.WithLabelValues(
		cc.index,
		packetTypeString(packetType),
		packetDropReasonString(reason),
	).Add(byteCountToPromCount(size))
}

func (cc *clientCollector) lostPackets(reason logging.PacketLossReason) {
	clientMetrics.lostPackets.WithLabelValues(cc.index, packetLossReasonString(reason)).Inc()
}

func (cc *clientCollector) updatedRTT(rtt *logging.RTTStats) {
	clientMetrics.minRTT.WithLabelValues(cc.index).Set(durationToPromGauge(rtt.MinRTT()))
	clientMetrics.latestRTT.WithLabelValues(cc.index).Set(durationToPromGauge(rtt.LatestRTT()))
	clientMetrics.smoothedRTT.WithLabelValues(cc.index).Set(durationToPromGauge(rtt.SmoothedRTT()))
}

func (cc *clientCollector) updateCongestionWindow(size logging.ByteCount) {
	clientMetrics.congestionWindow.WithLabelValues(cc.index).Set(float64(size))
}

func (cc *clientCollector) updatedCongestionState(state logging.CongestionState) {
	clientMetrics.congestionState.WithLabelValues(cc.index).Set(float64(state))
}

func (cc *clientCollector) updateMTU(mtu logging.ByteCount) {
	clientMetrics.mtu.WithLabelValues(cc.index).Set(float64(mtu))
	cc.logger.Debug().Msgf("QUIC MTU updated to %d", mtu)
}

func (cc *clientCollector) collectPackets(size logging.ByteCount, frames []logging.Frame, counter, bandwidth noopMetricVec, direction direction) {
	for _, frame := range frames {
		switch f := frame.(type) {
		case logging.DataBlockedFrame:
			cc.logger.Debug().Msgf("%s data_blocked frame", direction)
		case logging.StreamDataBlockedFrame:
			cc.logger.Debug().Int64("streamID", int64(f.StreamID)).Msgf("%s stream_data_blocked frame", direction)
		}
		counter.WithLabelValues(cc.index, frameName(frame)).Inc()
	}
	bandwidth.WithLabelValues(cc.index).Add(byteCountToPromCount(size))
}

func frameName(frame logging.Frame) string {
	if frame == nil {
		return "nil"
	} else {
		name := reflect.TypeOf(frame).Elem().Name()
		return strings.TrimSuffix(name, "Frame")
	}
}

type direction uint8

const (
	sent direction = iota
	received
)

func (d direction) String() string {
	if d == sent {
		return "sent"
	}
	return "received"
}
