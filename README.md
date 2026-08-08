# VentureChain

A permissioned proof-of-authority blockchain built in Go, serving as the native escrow layer for **Venturify** — replacing third-party escrow (Stripe Treasury) with an in-house ledger that natively understands investment contract milestones, disputes, and arbitration.

## Why a custom chain instead of a token on an existing chain

Venturify's escrow logic (lock → milestone release → dispute → arbitration) is business logic, not a generic transfer. Building it as native transaction types on a purpose-built chain — rather than a smart contract on a public chain — keeps the platform in full control of validator identity, block finality timing, and fee-free internal settlement, at the cost of not inheriting an existing chain's security/decentralization guarantees. That trade-off is appropriate for a permissioned platform-internal ledger, not a public currency.

## Architecture

- **Consensus:** Proof of Authority, round-robin block proposal across a fixed validator set defined in `config/genesis.json`
- **Native transaction types:** `MINT`, `TRANSFER`, `LOCK`, `RELEASE_MILESTONE`, `REFUND`, `FREEZE_DISPUTE`, `ARBITRATE` — escrow is enforced at the chain level, not via a smart contract VM
- **Storage:** Embedded BadgerDB per node, persisting blocks and world state across restarts
- **Networking:** HTTP+JSON gossip between validator nodes (`internal/p2p`)
- **Client API:** REST (`internal/api`), consumed by Venturify's Django backend — wallet balances, transaction submission, and the full escrow contract lifecycle
- **Escrow lifecycle:** Nine-state contract machine (`internal/escrow`) — DRAFT → PENDING_SIGNATURE → SIGNED → FUNDED → IN_PROGRESS ⇄ MILESTONE_REVIEW → COMPLETED, with DISPUTED and TERMINATED as exit paths

## Project layout

```
venturechain/
├── cmd/node/            entrypoint for a full validator node
├── cmd/walletcli/       wallet key generation and inspection
├── internal/blockchain/ block, transaction, state, validation
├── internal/consensus/  PoA engine, validator set, round-robin proposer
├── internal/wallet/     key management, transaction signing
├── internal/storage/    BadgerDB persistence
├── internal/mempool/    pending transaction pool
├── internal/escrow/     contract lifecycle, milestones, escrow tx builders
├── internal/p2p/        gossip networking, peer registry, block production loop
├── internal/api/        client-facing REST API
├── pkg/crypto/          secp256k1 signing, address derivation
└── config/genesis.json  chain config, validator set, initial allocations
```

## Running a single dev node

```powershell
go run .\cmd\node `
  --id validator-1 `
  --wallet validator1.wallet.json `
  --escrow-wallet escrow-authority.wallet.json `
  --p2p-bind :9001 `
  --p2p-public http://localhost:9001 `
  --api-bind :8001 `
  --internal-api-key dev-secret-change-me
```

## Running the 3-validator local demo

In three separate terminals:

```powershell
.\run-validator1.ps1
.\run-validator2.ps1
.\run-validator3.ps1
```

Each node persists to its own `data/validator-N` directory and gossips blocks/transactions to the other two over HTTP.

## REST API summary

| Endpoint | Purpose |
|---|---|
| `GET /api/wallet/{address}/balance` | Query VTC balance |
| `GET /api/wallet/{address}/nonce` | Query current nonce |
| `GET /api/chain/height` | Current chain height |
| `GET /api/chain/block/{index}` | Fetch a block |
| `POST /api/chain/tx` | Submit a signed transaction |
| `POST /api/chain/mint` | Mint VTC on confirmed fiat deposit (internal) |
| `POST /api/escrow/contracts` | Create an escrow contract |
| `GET /api/escrow/contracts/{id}` | Fetch contract state |
| `POST /api/escrow/contracts/{id}/advance` | Advance lifecycle state |
| `GET /api/escrow/contracts/{id}/funding-tx` | Get unsigned LOCK tx for investor to sign |
| `POST /api/escrow/contracts/{id}/confirm-funded` | Confirm on-chain lock matches contract |
| `POST /api/escrow/contracts/{id}/milestones/{mid}/submit` | Founder submits milestone |
| `POST /api/escrow/contracts/{id}/milestones/{mid}/dispute` | Investor disputes milestone |
| `POST /api/escrow/contracts/{id}/milestones/{mid}/release` | Release milestone funds (internal) |
| `POST /api/escrow/contracts/{id}/arbitrate` | Resolve a dispute (internal) |
| `POST /api/escrow/contracts/{id}/refund` | Refund remaining escrow (internal) |

Endpoints marked *(internal)* require an `X-Internal-Api-Key` header matching the node's `--internal-api-key`.

## Known limitations (devnet scope)

- Wallet private keys are stored in plaintext JSON — acceptable for local dev, not for production
- Contract metadata lives in-memory on each node (`internal/api/handlers_escrow.go`), not persisted to BadgerDB — a restart loses in-flight contract state even though chain state itself survives
- No block-range sync for a node that falls behind — a peer that misses gossip needs a manual resync path (documented as a known gap, candidate for a future phase)
- Static shared-secret auth on internal endpoints — fine for an FYP demo, not for a real deployment

## Author

Muaaz Tasawar (`MuaazTasawar`) — built as part of the Venturify FYP at COMSATS University Islamabad.