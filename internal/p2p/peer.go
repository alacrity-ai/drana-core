package p2p

import (
	"context"
	"log"
	"sync"
	"time"
)

// PeerManager manages connections to peers with dynamic discovery.
type PeerManager struct {
	mu            sync.RWMutex
	peers         map[string]*Client
	knownAddrs    map[string]bool
	selfAddr      string // listen address, to exclude self from connections
	advertiseAddr string // address shared via peer exchange
}

// NewPeerManager creates a peer manager. advertiseAddr is shared via peer exchange;
// if empty, falls back to selfListenAddr.
func NewPeerManager(endpoints map[string]string, selfListenAddr, advertiseAddr string) *PeerManager {
	if advertiseAddr == "" {
		advertiseAddr = selfListenAddr
	}
	pm := &PeerManager{
		peers:         make(map[string]*Client),
		knownAddrs:    make(map[string]bool),
		selfAddr:      selfListenAddr,
		advertiseAddr: advertiseAddr,
	}
	for _, addr := range endpoints {
		if addr != selfListenAddr && addr != advertiseAddr {
			pm.knownAddrs[addr] = true
		}
	}
	return pm
}

// Connect dials all known peers that aren't already connected.
func (pm *PeerManager) Connect(endpoints map[string]string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Add any new endpoints to known set.
	for _, addr := range endpoints {
		if addr != pm.selfAddr {
			pm.knownAddrs[addr] = true
		}
	}

	pm.connectToKnown()
	return nil
}

// connectToKnown dials all known addresses not yet connected. Caller must hold mu.
func (pm *PeerManager) connectToKnown() {
	for addr := range pm.knownAddrs {
		if addr == pm.selfAddr {
			continue
		}
		if _, connected := pm.peers[addr]; connected {
			continue
		}
		c, err := Dial(addr)
		if err != nil {
			continue
		}
		log.Printf("p2p: connected to %s", addr)
		pm.peers[addr] = c
	}
}

// Peers returns all currently connected peers.
func (pm *PeerManager) Peers() []*Client {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]*Client, 0, len(pm.peers))
	for _, c := range pm.peers {
		out = append(out, c)
	}
	return out
}

// KnownAddrs returns all known peer addresses (connected or not).
func (pm *PeerManager) KnownAddrs() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]string, 0, len(pm.knownAddrs))
	for addr := range pm.knownAddrs {
		out = append(out, addr)
	}
	return out
}

// AddPeer records a new peer address and connects if not already connected.
func (pm *PeerManager) AddPeer(addr string) {
	if addr == pm.selfAddr || addr == pm.advertiseAddr || addr == "" {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.knownAddrs[addr] {
		return // already known
	}
	pm.knownAddrs[addr] = true
	// Try to connect immediately.
	if _, connected := pm.peers[addr]; !connected {
		c, err := Dial(addr)
		if err != nil {
			return
		}
		log.Printf("p2p: discovered and connected to %s", addr)
		pm.peers[addr] = c
	}
}

// RunDiscovery periodically asks connected peers for their peer lists.
// This is how the network grows beyond the seed list.
func (pm *PeerManager) RunDiscovery(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.discoverOnce(ctx)
		}
	}
}

func (pm *PeerManager) discoverOnce(ctx context.Context) {
	peers := pm.Peers()
	myKnown := pm.KnownAddrs()

	for _, peer := range peers {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := peer.ExchangePeers(reqCtx, pm.advertiseAddr, myKnown)
		cancel()
		if err != nil {
			continue
		}
		for _, addr := range resp {
			pm.AddPeer(addr)
		}
	}

	// Also retry connecting to known but disconnected peers.
	pm.mu.Lock()
	pm.connectToKnown()
	pm.mu.Unlock()
}

// Close disconnects all peers.
func (pm *PeerManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, c := range pm.peers {
		c.Close()
	}
	pm.peers = make(map[string]*Client)
}
