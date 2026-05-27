package runtime

import "crypto/tls"

func testTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName: edgeServerNameHTTP2,
		MinVersion: tls.VersionTLS12,
	}
}

func testQUICTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName: edgeServerNameQUIC,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{edgeALPNQUIC},
	}
}
