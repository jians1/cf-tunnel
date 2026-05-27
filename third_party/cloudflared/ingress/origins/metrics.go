package origins

type Metrics interface {
	IncrementDNSUDPRequests()
	IncrementDNSTCPRequests()
}

type metrics struct{}

func (m *metrics) IncrementDNSUDPRequests() {}

func (m *metrics) IncrementDNSTCPRequests() {}

func NewMetrics(registerer any) Metrics {
	return &metrics{}
}
