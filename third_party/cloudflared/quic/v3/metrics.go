package v3

type DroppedReason int

const (
	DroppedWriteFailed DroppedReason = iota
	DroppedWriteDeadlineExceeded
	DroppedWriteFull
	DroppedWriteFlowUnknown
	DroppedReadFailed
	// Origin payloads that are too large to proxy.
	DroppedReadTooLarge
)

var droppedReason = map[DroppedReason]string{
	DroppedWriteFailed:           "write_failed",
	DroppedWriteDeadlineExceeded: "write_deadline_exceeded",
	DroppedWriteFull:             "write_full",
	DroppedWriteFlowUnknown:      "write_flow_unknown",
	DroppedReadFailed:            "read_failed",
	DroppedReadTooLarge:          "read_too_large",
}

func (dr DroppedReason) String() string {
	return droppedReason[dr]
}

type Metrics interface {
	IncrementFlows(connIndex uint8)
	DecrementFlows(connIndex uint8)
	FailedFlow(connIndex uint8)
	RetryFlowResponse(connIndex uint8)
	MigrateFlow(connIndex uint8)
	UnsupportedRemoteCommand(connIndex uint8, command string)
	DroppedUDPDatagram(connIndex uint8, reason DroppedReason)
	DroppedICMPPackets(connIndex uint8, reason DroppedReason)
}

type metrics struct{}

func (m *metrics) IncrementFlows(connIndex uint8) {}

func (m *metrics) DecrementFlows(connIndex uint8) {}

func (m *metrics) FailedFlow(connIndex uint8) {}

func (m *metrics) RetryFlowResponse(connIndex uint8) {}

func (m *metrics) MigrateFlow(connIndex uint8) {}

func (m *metrics) UnsupportedRemoteCommand(connIndex uint8, command string) {}

func (m *metrics) DroppedUDPDatagram(connIndex uint8, reason DroppedReason) {}

func (m *metrics) DroppedICMPPackets(connIndex uint8, reason DroppedReason) {}

func NewMetrics(registerer any) Metrics {
	return &metrics{}
}
