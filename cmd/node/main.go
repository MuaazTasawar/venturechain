package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/MuaazTasawar/venturechain/internal/api"
	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/MuaazTasawar/venturechain/internal/consensus"
	"github.com/MuaazTasawar/venturechain/internal/mempool"
	"github.com/MuaazTasawar/venturechain/internal/p2p"
	"github.com/MuaazTasawar/venturechain/internal/storage"
	"github.com/MuaazTasawar/venturechain/internal/wallet"
)

// genesisAllocation mirrors one entry of config/genesis.json's
// initial_allocations array.
type genesisAllocation struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
	Note    string `json:"note"`
}

type genesisFile struct {
	InitialAllocations []genesisAllocation `json:"initial_allocations"`
}

// loadInitialAllocations parses the genesis file's initial_allocations,
// treating the first entry as the platform treasury address by convention.
func loadInitialAllocations(path string) (map[string]int64, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read genesis file: %w", err)
	}
	var g genesisFile
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, "", fmt.Errorf("failed to parse genesis file: %w", err)
	}
	allocations := make(map[string]int64)
	treasuryAddr := ""
	for i, a := range g.InitialAllocations {
		allocations[a.Address] = a.Balance
		if i == 0 {
			treasuryAddr = a.Address
		}
	}
	if treasuryAddr == "" {
		return nil, "", fmt.Errorf("genesis file has no initial_allocations - a treasury address is required")
	}
	return allocations, treasuryAddr, nil
}

// joinNetwork announces this node to a bootstrap peer's /p2p/join endpoint.
func joinNetwork(bootstrapURL string, self *p2p.Peer) {
	body, err := json.Marshal(self)
	if err != nil {
		log.Printf("join: failed to marshal self: %v", err)
		return
	}
	resp, err := http.Post(bootstrapURL+"/p2p/join", "application/json", strings.NewReader(string(body)))
	if err != nil {
		log.Printf("join: failed to reach bootstrap %s: %v", bootstrapURL, err)
		return
	}
	defer resp.Body.Close()
	log.Printf("join: announced to %s, status %d", bootstrapURL, resp.StatusCode)
}

func main() {
	genesisPath := flag.String("genesis", "config/genesis.json", "path to genesis.json")
	dataDir := flag.String("data", "", "path to this node's data directory (default: data/<id>)")
	walletPath := flag.String("wallet", "", "path to this validator's .wallet.json (required)")
	escrowWalletPath := flag.String("escrow-wallet", "", "path to the escrow authority's .wallet.json (required)")
	validatorID := flag.String("id", "", "this node's validator id, e.g. validator-1 (required)")
	p2pBind := flag.String("p2p-bind", ":9001", "address to bind the p2p gossip server to")
	p2pPublic := flag.String("p2p-public", "", "public URL other nodes use to reach this node, e.g. http://192.168.1.10:9001 (required)")
	apiBind := flag.String("api-bind", ":8001", "address to bind the client-facing REST API to")
	peersFlag := flag.String("peers", "", "comma-separated bootstrap peers as id=url pairs, e.g. validator-2=http://192.168.1.11:9002")
	internalAPIKey := flag.String("internal-api-key", "", "shared secret required in X-Internal-Api-Key header for privileged endpoints (required)")
	flag.Parse()

	if *walletPath == "" || *validatorID == "" || *p2pPublic == "" || *internalAPIKey == "" || *escrowWalletPath == "" {
		log.Fatal("--wallet, --escrow-wallet, --id, --p2p-public, and --internal-api-key are all required. Run with -h for details.")
	}
	if *dataDir == "" {
		*dataDir = "data/" + *validatorID
	}

	validatorSet, err := consensus.LoadValidatorSetFromGenesis(*genesisPath)
	if err != nil {
		log.Fatalf("failed to load validator set: %v", err)
	}
	if !validatorSet.IsValidator(*validatorID) {
		log.Fatalf("id %s is not a configured validator in %s", *validatorID, *genesisPath)
	}

	localKey, err := wallet.Load(*walletPath)
	if err != nil {
		log.Fatalf("failed to load validator wallet: %v", err)
	}

	escrowKey, err := wallet.Load(*escrowWalletPath)
	if err != nil {
		log.Fatalf("failed to load escrow authority wallet: %v", err)
	}
	escrowWallet := wallet.New(escrowKey)

	allocations, treasuryAddr, err := loadInitialAllocations(*genesisPath)
	if err != nil {
		log.Fatalf("failed to load initial allocations: %v", err)
	}

	store, err := storage.Open(*dataDir)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}
	defer store.Close()

	height, err := store.LoadHeight()
	if err != nil {
		log.Fatalf("failed to read chain height from storage: %v", err)
	}

	var chain *blockchain.Blockchain
	if height < 0 {
		log.Println("no existing chain found, initializing from genesis")
		chain = blockchain.NewBlockchain(allocations)
		if err := store.SaveBlock(chain.LatestBlock()); err != nil {
			log.Fatalf("failed to persist genesis block: %v", err)
		}
		if err := store.SaveState(chain.State); err != nil {
			log.Fatalf("failed to persist genesis state: %v", err)
		}
	} else {
		log.Printf("existing chain found at height %d, loading from storage", height)
		blocks, err := store.LoadAllBlocks()
		if err != nil {
			log.Fatalf("failed to load blocks from storage: %v", err)
		}
		state, err := store.LoadState()
		if err != nil {
			log.Fatalf("failed to load state from storage: %v", err)
		}
		chain = &blockchain.Blockchain{Blocks: blocks, State: state}
		if err := chain.ValidateChain(); err != nil {
			log.Fatalf("loaded chain failed validation: %v", err)
		}
	}

	mp := mempool.New()
	engine := consensus.NewEngine(validatorSet, *validatorID, localKey.PrivateKey, chain, mp)

	self := &p2p.Peer{ID: *validatorID, Address: *p2pPublic}
	node := p2p.NewNode(self, chain, mp, engine, store)

	if *peersFlag != "" {
		for _, pair := range strings.Split(*peersFlag, ",") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				log.Printf("skipping malformed --peers entry: %s (expected id=url)", pair)
				continue
			}
			peer := &p2p.Peer{ID: parts[0], Address: parts[1]}
			node.Peers.Add(peer)
			go joinNetwork(peer.Address, self)
		}
	}

	go func() {
		if err := node.Start(*p2pBind); err != nil {
			log.Fatalf("p2p server failed: %v", err)
		}
	}()

	go node.RunProductionLoop()

	server := api.NewServer(chain, mp, node.Peers, treasuryAddr, escrowWallet, *internalAPIKey)
	log.Printf("api: node %s serving client API on %s", *validatorID, *apiBind)
	if err := http.ListenAndServe(*apiBind, server.Router()); err != nil {
		log.Fatalf("api server failed: %v", err)
	}
}