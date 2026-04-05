package p2p

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	pb "github.com/drana-chain/drana/internal/proto/pb"
)

// mockHandler is a minimal handler for testing peer exchange.
type mockHandler struct {
	pb.UnimplementedConsensusServiceServer
}

func (m *mockHandler) OnProposal(ctx context.Context, req *pb.BlockProposal) (*pb.BlockVote, error) {
	return &pb.BlockVote{}, nil
}
func (m *mockHandler) OnFinalizedBlock(ctx context.Context, fb *pb.FinalizedBlock) (*pb.PeerStatus, error) {
	return &pb.PeerStatus{}, nil
}
func (m *mockHandler) OnSyncRequest(ctx context.Context, from, to uint64) (*pb.SyncResponse, error) {
	return &pb.SyncResponse{}, nil
}
func (m *mockHandler) OnGetStatus(ctx context.Context) (*pb.PeerStatus, error) {
	return &pb.PeerStatus{}, nil
}
func (m *mockHandler) OnSubmitTx(ctx context.Context, tx *pb.TxSubmission) (*pb.TxSubmissionResponse, error) {
	return &pb.TxSubmissionResponse{}, nil
}

func startTestServer(t *testing.T, addr string, pm *PeerManager) *Server {
	t.Helper()
	s := NewServer(addr, &mockHandler{}, pm)
	if err := s.Start(); err != nil {
		t.Fatalf("start server %s: %v", addr, err)
	}
	t.Cleanup(func() { s.Stop() })
	return s
}

func getFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestPeerExchangeDiscovery(t *testing.T) {
	// Three nodes: A, B, C.
	// A knows B. B knows A and C. C knows B.
	// After peer exchange, A should discover C.

	addrA := getFreePort(t)
	addrB := getFreePort(t)
	addrC := getFreePort(t)

	pmA := NewPeerManager(map[string]string{"B": addrB}, addrA, addrA)
	pmB := NewPeerManager(map[string]string{"A": addrA, "C": addrC}, addrB, addrB)
	pmC := NewPeerManager(map[string]string{"B": addrB}, addrC, addrC)

	startTestServer(t, addrA, pmA)
	startTestServer(t, addrB, pmB)
	startTestServer(t, addrC, pmC)
	time.Sleep(200 * time.Millisecond)

	// Connect each to their seeds.
	pmA.Connect(map[string]string{"B": addrB})
	pmB.Connect(map[string]string{"A": addrA, "C": addrC})
	pmC.Connect(map[string]string{"B": addrB})
	time.Sleep(200 * time.Millisecond)

	// Before discovery: A knows only B.
	if len(pmA.Peers()) != 1 {
		t.Fatalf("A should have 1 peer before discovery, got %d", len(pmA.Peers()))
	}

	// Run one round of discovery on A.
	ctx := context.Background()
	pmA.discoverOnce(ctx)
	time.Sleep(200 * time.Millisecond)

	// After discovery: A should know B and C.
	knownA := pmA.KnownAddrs()
	foundC := false
	for _, addr := range knownA {
		if addr == addrC {
			foundC = true
		}
	}
	if !foundC {
		t.Fatalf("A should have discovered C, known addrs: %v", knownA)
	}

	// A should now be connected to C.
	if len(pmA.Peers()) != 2 {
		t.Fatalf("A should have 2 peers after discovery, got %d", len(pmA.Peers()))
	}
}

func TestPeerManagerExcludesSelf(t *testing.T) {
	addr := getFreePort(t)
	pm := NewPeerManager(map[string]string{"self": addr}, addr, addr)

	// Should not add self.
	pm.AddPeer(addr)
	if len(pm.KnownAddrs()) != 0 {
		t.Fatalf("should exclude self, got %v", pm.KnownAddrs())
	}
}

func TestPeerManagerDedup(t *testing.T) {
	addrA := getFreePort(t)
	pm := NewPeerManager(nil, "127.0.0.1:0", "")

	pm.AddPeer(addrA)
	pm.AddPeer(addrA) // duplicate
	if len(pm.KnownAddrs()) != 1 {
		t.Fatalf("should dedup, got %d known", len(pm.KnownAddrs()))
	}
}

// Verify the gRPC ExchangePeers method works directly.
func TestExchangePeersRPC(t *testing.T) {
	addr := getFreePort(t)
	pm := NewPeerManager(map[string]string{"peer1": "1.2.3.4:26601"}, addr, addr)
	startTestServer(t, addr, pm)
	time.Sleep(200 * time.Millisecond)

	// Connect a client and call ExchangePeers.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(nil),
		grpc.WithDefaultCallOptions())
	if err != nil {
		// Use the Dial function instead.
		c, err := Dial(addr)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()

		peers, err := c.ExchangePeers(context.Background(), "5.6.7.8:26601", nil)
		if err != nil {
			t.Fatalf("ExchangePeers: %v", err)
		}

		// Should return the seed peer.
		found := false
		for _, p := range peers {
			if p == "1.2.3.4:26601" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected seed peer in response, got %v", peers)
		}

		// The server should have learned about us.
		known := pm.KnownAddrs()
		foundUs := false
		for _, a := range known {
			if a == "5.6.7.8:26601" {
				foundUs = true
			}
		}
		if !foundUs {
			t.Fatalf("server should have learned our address, known: %v", known)
		}
		return
	}
	defer conn.Close()
}
