package blockchain

import (
	"fmt"
)

// ValidateBlock performs structural and authority checks on a proposed block
// before its transactions are ever applied to state.
func ValidateBlock(block *Block, prevBlock *Block, validatorPubKeyHex string) error {
	if block.Index != prevBlock.Index+1 {
		return fmt.Errorf("expected index %d, got %d", prevBlock.Index+1, block.Index)
	}
	if block.PrevHash != prevBlock.Hash {
		return fmt.Errorf("prev_hash does not match latest block hash")
	}
	if block.calculateHash() != block.Hash {
		return fmt.Errorf("block hash does not match computed hash")
	}
	if block.calculateMerkleRoot() != block.MerkleRoot {
		return fmt.Errorf("merkle root does not match transaction set")
	}
	valid, err := block.VerifySignature(validatorPubKeyHex)
	if err != nil {
		return fmt.Errorf("signature verification error: %w", err)
	}
	if !valid {
		return fmt.Errorf("block signature invalid for validator %s", block.ValidatorID)
	}
	return nil
}

// ValidateTransaction checks a transaction's signature, nonce ordering, and
// type-specific escrow business rules against the current state, without
// mutating it. Call ApplyTransaction afterwards to commit the effect.
func ValidateTransaction(tx *Transaction, state *State) error {
	valid, err := tx.VerifySignature()
	if err != nil {
		return fmt.Errorf("signature check failed: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid signature")
	}

	if tx.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	switch tx.Type {
	case TxMint:
		// Only the platform treasury may mint - enforced by the API layer
		// checking the caller's role before a MINT tx ever reaches the
		// mempool. At chain level we just require a valid recipient.
		if tx.To == "" {
			return fmt.Errorf("mint requires a recipient")
		}
		return nil

	case TxTransfer:
		return checkSenderFunds(tx, state)

	case TxLock:
		if tx.ContractID == "" {
			return fmt.Errorf("lock requires a contract_id")
		}
		if existing := state.GetLock(tx.ContractID); existing != nil {
			return fmt.Errorf("contract %s already has an active escrow lock", tx.ContractID)
		}
		return checkSenderFunds(tx, state)

	case TxReleaseMilestone:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" {
			return ErrEscrowNotLocked
		}
		if tx.Amount > lock.Amount {
			return fmt.Errorf("release amount %d exceeds remaining escrow %d", tx.Amount, lock.Amount)
		}
		if tx.To != lock.Founder {
			return fmt.Errorf("milestone release must pay the founder on record")
		}
		return nil

	case TxRefund:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" && lock.Status != "FROZEN" {
			return fmt.Errorf("escrow must be locked or frozen to refund")
		}
		if tx.To != lock.Investor {
			return fmt.Errorf("refund must pay the investor on record")
		}
		return nil

	case TxFreezeDispute:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" {
			return ErrEscrowNotLocked
		}
		return nil

	case TxArbitrate:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "FROZEN" {
			return ErrEscrowNotFrozen
		}
		if tx.Amount > lock.Amount {
			return fmt.Errorf("arbitrated amount %d exceeds remaining escrow %d", tx.Amount, lock.Amount)
		}
		if tx.To != lock.Founder && tx.To != lock.Investor {
			return fmt.Errorf("arbitration must pay either the founder or investor on record")
		}
		return nil

	default:
		return fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
}

func checkSenderFunds(tx *Transaction, state *State) error {
	if tx.From == "" {
		return fmt.Errorf("transaction has no sender address")
	}
	balance := state.GetBalance(tx.From)
	if balance < tx.Amount {
		return ErrInsufficientBalance
	}
	expectedNonce := state.GetNonce(tx.From)
	if tx.Nonce != expectedNonce {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidNonce, expectedNonce, tx.Nonce)
	}
	return nil
}

// ApplyTransaction mutates state according to a transaction that has already
// passed ValidateTransaction. It must never be called without a prior
// successful validation.
func ApplyTransaction(tx *Transaction, state *State) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	switch tx.Type {
	case TxMint:
		state.Balances[tx.To] += tx.Amount

	case TxTransfer:
		state.Balances[tx.From] -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		state.Nonces[tx.From]++

	case TxLock:
		state.Balances[tx.From] -= tx.Amount
		state.Locked[tx.ContractID] = &EscrowLock{
			ContractID: tx.ContractID,
			Investor:   tx.From,
			Founder:    tx.To,
			Amount:     tx.Amount,
			Status:     "LOCKED",
		}
		state.Nonces[tx.From]++

	case TxReleaseMilestone:
		lock := state.Locked[tx.ContractID]
		lock.Amount -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		if lock.Amount == 0 {
			lock.Status = "RELEASED"
		}

	case TxRefund:
		lock := state.Locked[tx.ContractID]
		refundAmount := lock.Amount
		state.Balances[tx.To] += refundAmount
		lock.Amount = 0
		lock.Status = "REFUNDED"

	case TxFreezeDispute:
		lock := state.Locked[tx.ContractID]
		lock.Status = "FROZEN"

	case TxArbitrate:
		lock := state.Locked[tx.ContractID]
		lock.Amount -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		if lock.Amount == 0 {
			lock.Status = "RELEASED"
		} else {
			lock.Status = "LOCKED"
		}

	default:
		return fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
	return nil
}