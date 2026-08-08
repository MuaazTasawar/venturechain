go run .\cmd\node `
  --id validator-1 `
  --wallet validator1.wallet.json `
  --escrow-wallet escrow-authority.wallet.json `
  --data data/validator-1 `
  --p2p-bind :9001 `
  --p2p-public http://localhost:9001 `
  --api-bind :8001 `
  --peers "validator-2=http://localhost:9002,validator-3=http://localhost:9003" `
  --internal-api-key dev-secret-change-me