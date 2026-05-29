package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sync"

	quicgo "github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
)

type quicDialOpts struct {
	skipPortReuse bool
}

var (
	portForConnIndex = make(map[uint8]int)
	portMapMutex     sync.Mutex
)

func dialQUIC(
	ctx context.Context,
	quicConfig *quicgo.Config,
	tlsConfig *tls.Config,
	edgeAddr netip.AddrPort,
	localAddr net.IP,
	connIndex uint8,
	logger *zerolog.Logger,
	opts quicDialOpts,
) (quicgo.Connection, error) {
	udpConn, err := createUDPConnForConnIndex(connIndex, localAddr, edgeAddr, opts, logger)
	if err != nil {
		return nil, err
	}

	conn, err := quicgo.Dial(ctx, udpConn, net.UDPAddrFromAddrPort(edgeAddr), tlsConfig, quicConfig)
	if err != nil {
		_ = udpConn.Close()
		return nil, fmt.Errorf("dial quic edge: %w", err)
	}

	return &wrapCloseableConnQUICConnection{
		Connection: conn,
		udpConn:    udpConn,
	}, nil
}

func createUDPConnForConnIndex(connIndex uint8, localIP net.IP, edgeIP netip.AddrPort, opts quicDialOpts, logger *zerolog.Logger) (*net.UDPConn, error) {
	listenNetwork := "udp"
	if runtime.GOOS == "darwin" {
		if edgeIP.Addr().Is4() {
			listenNetwork = "udp4"
		} else {
			listenNetwork = "udp6"
		}
	}

	if opts.skipPortReuse {
		return net.ListenUDP(listenNetwork, &net.UDPAddr{IP: localIP, Port: 0})
	}

	portMapMutex.Lock()
	defer portMapMutex.Unlock()

	if port, ok := portForConnIndex[connIndex]; ok {
		udpConn, err := net.ListenUDP(listenNetwork, &net.UDPAddr{IP: localIP, Port: port})
		if err == nil {
			return udpConn, nil
		}
		logger.Debug().Err(err).Msgf("Unable to reuse port %d for connIndex %d. Falling back to random allocation.", port, connIndex)
	}

	udpConn, err := net.ListenUDP(listenNetwork, &net.UDPAddr{IP: localIP, Port: 0})
	if err == nil {
		udpAddr, ok := udpConn.LocalAddr().(*net.UDPAddr)
		if !ok {
			return nil, fmt.Errorf("unable to cast local addr to udp")
		}
		portForConnIndex[connIndex] = udpAddr.Port
	} else {
		delete(portForConnIndex, connIndex)
	}

	return udpConn, err
}

type wrapCloseableConnQUICConnection struct {
	quicgo.Connection
	udpConn *net.UDPConn
}

func (w *wrapCloseableConnQUICConnection) CloseWithError(errorCode quicgo.ApplicationErrorCode, reason string) error {
	err := w.Connection.CloseWithError(errorCode, reason)
	_ = w.udpConn.Close()
	return err
}
