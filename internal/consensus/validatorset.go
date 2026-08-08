package consensus

import (
	"encoding/json"
	"fmt"
	"os"
)

// Validator is a single authority node permitted to propose and sign blocks.
type Validator struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
}

// genesisFile mirrors the relevant fields of config/genesis.json needed to
// build the validator set at startup.
type genesisFile struct {
	ChainID          string      `json:"chain_id"`
	BlockTimeSeconds int64       `json:"block_time_seconds"`
	Validators       []Validator `json:"validators"`
}

// ValidatorSet holds the ordered list of authorities for round-robin PoA
// block production.
type ValidatorSet struct {
	ChainID          string
	BlockTimeSeconds int64
	Validators       []Validator
}

// LoadValidatorSetFromGenesis reads config/genesis.json and builds the
// validator set used for round-robin proposer selection.
func LoadValidatorSetFromGenesis(path string) (*ValidatorSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis file: %w", err)
	}
	var g genesisFile
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("failed to parse genesis file: %w", err)
	}
	if len(g.Validators) == 0 {
		return nil, fmt.Errorf("genesis file has no validators configured")
	}
	return &ValidatorSet{
		ChainID:          g.ChainID,
		BlockTimeSeconds: g.BlockTimeSeconds,
		Validators:       g.Validators,
	}, nil
}

// ProposerForHeight returns the validator responsible for proposing the
// block at the given height, using simple round-robin rotation.
func (vs *ValidatorSet) ProposerForHeight(height int64) Validator {
	idx := int(height) % len(vs.Validators)
	return vs.Validators[idx]
}

// PublicKeyFor looks up a validator's public key by ID, used when verifying
// a received block's signature.
func (vs *ValidatorSet) PublicKeyFor(validatorID string) (string, error) {
	for _, v := range vs.Validators {
		if v.ID == validatorID {
			return v.PublicKey, nil
		}
	}
	return "", fmt.Errorf("unknown validator id: %s", validatorID)
}

// IsValidator reports whether the given ID belongs to a configured
// authority.
func (vs *ValidatorSet) IsValidator(validatorID string) bool {
	_, err := vs.PublicKeyFor(validatorID)
	return err == nil
}