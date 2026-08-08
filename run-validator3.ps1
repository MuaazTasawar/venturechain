go run .\cmd\node `
  --id validator-3 `
  --wallet validator3.wallet.json `
  --escrow-wallet escrow-authority.wallet.json `
  --data data/validator-3 `
  --p2p-bind :9003 `
  --p2p-public http://localhost:9003 `
  --api-bind :8003 `
  --peers "validator-1=http://localhost:9001,validator-2=http://localhost:9002" `
  --internal-api-key dev-secret-change-me