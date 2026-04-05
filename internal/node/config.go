package node

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the runtime configuration for a validator node.
type Config struct {
	GenesisPath   string            `json:"genesisPath"`
	DataDir       string            `json:"dataDir"`
	PrivKeyHex    string            `json:"privKeyHex"`
	ListenAddr     string            `json:"listenAddr"`
	AdvertiseAddr  string            `json:"advertiseAddr,omitempty"` // public address shared via peer exchange (defaults to listenAddr)
	RPCListenAddr  string            `json:"rpcListenAddr,omitempty"` // JSON HTTP RPC (e.g., "0.0.0.0:26657")
	PeerEndpoints  map[string]string `json:"peerEndpoints"`          // validator name -> "host:port"
}

// LoadConfig reads a node config from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
