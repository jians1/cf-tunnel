package metrics

const (
	Cloudflared = "cloudflared"
)

const (
	ConfigurationManager = "config"

	OperationUpdateConfiguration = "update_configuration"
)

const (
	SessionManager = "session"

	OperationRegisterUdpSession   = "register_udp_session"
	OperationUnregisterUdpSession = "unregister_udp_session"
)

const (
	Registration = "registration"

	OperationRegisterConnection       = "register_connection"
	OperationUnregisterConnection     = "unregister_connection"
	OperationUpdateLocalConfiguration = "update_local_configuration"
)

type rpcMetrics struct {
	ClientOperations noopMetricVec
	ClientFailures   noopMetricVec
}

type noopMetric struct{}

func (noopMetric) Inc() {}

type noopMetricVec struct{}

func (noopMetricVec) WithLabelValues(labels ...string) noopMetric {
	return noopMetric{}
}

type Timer struct{}

func (Timer) ObserveDuration() {}

var CapnpMetrics = &rpcMetrics{}

func ObserveServerHandler(inner func() error, handler, method string) error {
	return inner()
}

func NewClientOperationLatencyObserver(server string, method string) Timer {
	return Timer{}
}
