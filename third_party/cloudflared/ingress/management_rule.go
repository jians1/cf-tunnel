package ingress

// NewManagementRule builds an internal ingress rule for a local management HTTP handler.
func NewManagementRule(hostname string, proxy HTTPLocalProxy) Rule {
	return Rule{
		Hostname: hostname,
		Service:  newManagementService(proxy),
	}
}
