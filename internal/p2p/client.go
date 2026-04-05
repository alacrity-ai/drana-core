package p2p

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "github.com/drana-chain/drana/internal/proto/pb"
)

// Client wraps a gRPC connection to a single peer.
type Client struct {
	conn   *grpc.ClientConn
	client pb.ConsensusServiceClient
	Addr   string
}

func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{
		conn:   conn,
		client: pb.NewConsensusServiceClient(conn),
		Addr:   addr,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ProposeBlock(ctx context.Context, proposal *pb.BlockProposal) (*pb.BlockVote, error) {
	return c.client.ProposeBlock(ctx, proposal)
}

func (c *Client) NotifyFinalized(ctx context.Context, fb *pb.FinalizedBlock) (*pb.PeerStatus, error) {
	return c.client.NotifyFinalizedBlock(ctx, fb)
}

func (c *Client) SyncBlocks(ctx context.Context, from, to uint64) (*pb.SyncResponse, error) {
	return c.client.SyncBlocks(ctx, &pb.SyncRequest{FromHeight: from, ToHeight: to})
}

func (c *Client) GetStatus(ctx context.Context) (*pb.PeerStatus, error) {
	return c.client.GetStatus(ctx, &pb.PeerStatus{})
}

func (c *Client) SubmitTx(ctx context.Context, tx *pb.TxSubmission) (*pb.TxSubmissionResponse, error) {
	return c.client.SubmitTx(ctx, tx)
}

func (c *Client) ExchangePeers(ctx context.Context, myAddr string, myKnown []string) ([]string, error) {
	resp, err := c.client.ExchangePeers(ctx, &pb.PeerExchangeRequest{
		SenderAddr: myAddr,
		KnownPeers: myKnown,
	})
	if err != nil {
		return nil, err
	}
	return resp.Peers, nil
}
