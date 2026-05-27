package runtime

func CloneSessionWithProtocol(session Session, protocol string) Session {
	cloned := session
	cloned.Edge.Protocol = protocol
	return cloned
}
