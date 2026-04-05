package p2p

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	pb "github.com/drana-chain/drana/internal/proto/pb"
)

// Handler is implemented by the consensus engine to handle incoming RPCs.
type Handler interface {
	OnProposal(ctx context.Context, proposal *pb.BlockProposal) (*pb.BlockVote, error)
	OnFinalizedBlock(ctx context.Context, fb *pb.FinalizedBlock) (*pb.PeerStatus, error)
	OnSyncRequest(ctx context.Context, from, to uint64) (*pb.SyncResponse, error)
	OnGetStatus(ctx context.Context) (*pb.PeerStatus, error)
	OnSubmitTx(ctx context.Context, tx *pb.TxSubmission) (*pb.TxSubmissionResponse, error)
}

// Server wraps a gRPC server for validator-to-validator communication.
type Server struct {
	pb.UnimplementedConsensusServiceServer
	handler    Handler
	peers      *PeerManager
	grpcServer *grpc.Server
	listenAddr string
}

func NewServer(listenAddr string, handler Handler, peers *PeerManager) *Server {
	return &Server{
		handler:    handler,
		peers:      peers,
		listenAddr: listenAddr,
	}
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.listenAddr, err)
	}
	s.grpcServer = grpc.NewServer()
	pb.RegisterConsensusServiceServer(s.grpcServer, s)
	log.Printf("p2p: listening on %s", s.listenAddr)
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("p2p: serve error: %v", err)
		}
	}()
	return nil
}

func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

func (s *Server) ProposeBlock(ctx context.Context, req *pb.BlockProposal) (*pb.BlockVote, error) {
	return s.handler.OnProposal(ctx, req)
}

func (s *Server) NotifyFinalizedBlock(ctx context.Context, req *pb.FinalizedBlock) (*pb.PeerStatus, error) {
	return s.handler.OnFinalizedBlock(ctx, req)
}

func (s *Server) SyncBlocks(ctx context.Context, req *pb.SyncRequest) (*pb.SyncResponse, error) {
	return s.handler.OnSyncRequest(ctx, req.FromHeight, req.ToHeight)
}

func (s *Server) GetStatus(ctx context.Context, req *pb.PeerStatus) (*pb.PeerStatus, error) {
	return s.handler.OnGetStatus(ctx)
}

func (s *Server) SubmitTx(ctx context.Context, req *pb.TxSubmission) (*pb.TxSubmissionResponse, error) {
	return s.handler.OnSubmitTx(ctx, req)
}

func (s *Server) ExchangePeers(ctx context.Context, req *pb.PeerExchangeRequest) (*pb.PeerExchangeResponse, error) {
	// Learn about the requester.
	if req.SenderAddr != "" {
		s.peers.AddPeer(req.SenderAddr)
	}
	// Learn about peers the requester knows.
	for _, addr := range req.KnownPeers {
		s.peers.AddPeer(addr)
	}
	// Return our known peers.
	return &pb.PeerExchangeResponse{
		Peers: s.peers.KnownAddrs(),
	}, nil
}
