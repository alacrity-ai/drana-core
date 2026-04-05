package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/drana-chain/drana/internal/consensus"
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/genesis"
	"github.com/drana-chain/drana/internal/mempool"
	"github.com/drana-chain/drana/internal/p2p"
	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/store"
	"github.com/drana-chain/drana/internal/types"
)

// Node is the top-level wiring for a DRANA validator node.
type Node struct {
	Config     *Config
	Genesis    *types.GenesisConfig
	Engine     *consensus.Engine
	P2PServer  *p2p.Server
	RPCServer  *rpc.Server
	Peers      *p2p.PeerManager
	BlockStore *store.BlockStore
	KVStore    *store.KVStore
}

// NewNode creates and initializes a DRANA node.
func NewNode(cfg *Config) (*Node, error) {
	// Load genesis.
	genCfg, err := genesis.LoadGenesis(cfg.GenesisPath)
	if err != nil {
		return nil, fmt.Errorf("load genesis: %w", err)
	}

	// Derive identity.
	privBytes, err := hex.DecodeString(cfg.PrivKeyHex)
	if err != nil || len(privBytes) != 64 {
		return nil, fmt.Errorf("invalid private key hex")
	}
	var privKey crypto.PrivateKey
	copy(privKey[:], privBytes)
	var pubKey crypto.PublicKey
	copy(pubKey[:], privKey[32:])
	address := crypto.AddressFromPublicKey(pubKey)

	// Verify this node is a genesis validator.
	found := false
	for _, v := range genCfg.Validators {
		if v.Address == address {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("private key does not correspond to a genesis validator")
	}

	// Open stores.
	kvPath := filepath.Join(cfg.DataDir, "state")
	kv, err := store.OpenKVStore(kvPath)
	if err != nil {
		return nil, fmt.Errorf("open kvstore: %w", err)
	}

	bsPath := filepath.Join(cfg.DataDir, "blocks")
	bs, err := store.OpenBlockStore(bsPath)
	if err != nil {
		kv.Close()
		return nil, fmt.Errorf("open blockstore: %w", err)
	}

	// Load or initialize state.
	ws, err := kv.LoadState()
	if err != nil {
		kv.Close()
		bs.Close()
		return nil, fmt.Errorf("load state: %w", err)
	}

	if ws.GetChainHeight() == 0 && len(ws.AllAccounts()) == 0 {
		// Fresh start — initialize from genesis.
		ws, err = genesis.InitializeState(genCfg)
		if err != nil {
			kv.Close()
			bs.Close()
			return nil, fmt.Errorf("initialize state: %w", err)
		}
		if err := kv.SaveState(ws); err != nil {
			kv.Close()
			bs.Close()
			return nil, fmt.Errorf("save initial state: %w", err)
		}
		log.Printf("node: initialized from genesis")
	} else {
		log.Printf("node: loaded state at height %d", ws.GetChainHeight())
	}

	// Load last block if we have state.
	var lastBlock *types.Block
	if ws.GetChainHeight() > 0 {
		lastBlock, err = bs.GetBlockByHeight(ws.GetChainHeight())
		if err != nil {
			log.Printf("node: warning: could not load last block: %v", err)
		}
	}

	// Merge seed sources: config + genesis + hardcoded defaults.
	allSeeds := make(map[string]string)
	for name, addr := range cfg.PeerEndpoints {
		allSeeds[name] = addr
	}
	for i, addr := range genCfg.SeedNodes {
		key := fmt.Sprintf("genesis-seed-%d", i)
		if _, exists := allSeeds[key]; !exists {
			allSeeds[key] = addr
		}
	}
	for i, addr := range DefaultSeedNodes {
		key := fmt.Sprintf("default-seed-%d", i)
		if _, exists := allSeeds[key]; !exists {
			allSeeds[key] = addr
		}
	}

	// Create components.
	mp := mempool.New(10000)
	peers := p2p.NewPeerManager(allSeeds, cfg.ListenAddr, cfg.AdvertiseAddr)

	engine := consensus.NewEngine(genCfg, genCfg.Validators, privKey, address, ws, mp, bs, kv, peers)
	engine.SetLastBlock(lastBlock)

	p2pServer := p2p.NewServer(cfg.ListenAddr, engine, peers)

	var rpcServer *rpc.Server
	if cfg.RPCListenAddr != "" {
		rpcServer = rpc.NewServer(cfg.RPCListenAddr, engine, bs, genCfg)
	}

	return &Node{
		Config:     cfg,
		Genesis:    genCfg,
		Engine:     engine,
		P2PServer:  p2pServer,
		RPCServer:  rpcServer,
		Peers:      peers,
		BlockStore: bs,
		KVStore:    kv,
	}, nil
}

// Start begins the node: starts P2P server, connects to peers, syncs, and runs consensus.
func (n *Node) Start(ctx context.Context) error {
	// Start gRPC server.
	if err := n.P2PServer.Start(); err != nil {
		return fmt.Errorf("start p2p: %w", err)
	}

	// Start JSON HTTP RPC server.
	if n.RPCServer != nil {
		if err := n.RPCServer.Start(); err != nil {
			return fmt.Errorf("start rpc: %w", err)
		}
	}

	// Connect to seed peers (best-effort — some may not be up yet).
	// Seeds are already loaded into the PeerManager from config + genesis + defaults.
	if err := n.Peers.Connect(nil); err != nil {
		log.Printf("node: peer connection: %v (will retry during discovery)", err)
	}

	// Start peer discovery in the background.
	go n.Peers.RunDiscovery(ctx, 30*time.Second)

	// Sync to network.
	if err := n.Engine.SyncToNetwork(ctx); err != nil {
		log.Printf("node: sync error: %v", err)
	}

	// Run consensus loop.
	return n.Engine.Run(ctx)
}

// Stop gracefully shuts down the node.
func (n *Node) Stop() error {
	if n.RPCServer != nil {
		n.RPCServer.Stop(context.Background())
	}
	n.P2PServer.Stop()
	n.Peers.Close()
	if err := n.KVStore.SaveState(n.Engine.CurrentState()); err != nil {
		log.Printf("node: save state on shutdown: %v", err)
	}
	n.KVStore.Close()
	n.BlockStore.Close()
	log.Printf("node: stopped at height %d", n.Engine.CurrentHeight())
	return nil
}
