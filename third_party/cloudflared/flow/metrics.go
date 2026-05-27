package flow

type noopMetric struct{}

func (noopMetric) Inc() {}

type noopMetricVec struct{}

func (noopMetricVec) WithLabelValues(labels ...string) noopMetric {
	return noopMetric{}
}

var flowRegistrationsDropped noopMetricVec
