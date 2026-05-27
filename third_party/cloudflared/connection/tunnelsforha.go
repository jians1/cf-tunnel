package connection

import (
	"fmt"
	"sync"
)

// tunnelsForHA maps this cloudflared instance's HA connections to the tunnel IDs they serve.
type tunnelsForHA struct {
	sync.Mutex
	entries map[uint8]string
}

// NewTunnelsForHA initializes the Prometheus metrics etc for a tunnelsForHA.
func newTunnelsForHA() tunnelsForHA {
	return tunnelsForHA{
		entries: make(map[uint8]string),
	}
}

// Track a new tunnel ID, removing the disconnected tunnel (if any) and update metrics.
func (t *tunnelsForHA) AddTunnelID(haConn uint8, tunnelID string) {
	t.Lock()
	defer t.Unlock()
	haStr := fmt.Sprintf("%v", haConn)
	t.entries[haConn] = tunnelID
	_ = haStr
}

func (t *tunnelsForHA) String() string {
	t.Lock()
	defer t.Unlock()
	return fmt.Sprintf("%v", t.entries)
}
