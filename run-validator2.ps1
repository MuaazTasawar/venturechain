go run .\cmd\node `
  --id validator-2 `
  --wallet validator2.wallet.json `
  --escrow-wallet escrow-authority.wallet.json `
  --data data/validator-2 `
  --p2p-bind :9002 `
  --p2p-public http://localhost:9002 `
  --api-bind :8002 `
  --peers "validator-1=http://localhost:9001,validator-3=http://localhost:9003" `
  --internal-api-key dev-secret-change-me