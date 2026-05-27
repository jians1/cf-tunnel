package runtime

import "io"

type ControlStreamError struct{}

func (e *ControlStreamError) Error() string {
	return "control stream encountered a failure while serving"
}

type StreamListenerError struct{}

func (e *StreamListenerError) Error() string {
	return "accept stream listener encountered a failure while serving"
}

type DatagramManagerError struct{}

func (e *DatagramManagerError) Error() string {
	return "datagram manager encountered a failure while serving"
}

type nopCloserReadWriter struct {
	io.ReadWriteCloser
}

func (rw *nopCloserReadWriter) Close() error {
	return nil
}
