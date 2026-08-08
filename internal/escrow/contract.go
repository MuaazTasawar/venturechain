package escrow

import (
	"fmt"
	"time"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
)

// Contract is the escrow-layer representation of a Venturify investment
// deal - the thing the Django backend creates, and this chain enforces.
type Contract struct {
	ID              string       `json:"id"`
	InvestorAddress string       `json:"investor_address"`
	FounderAddress  string       `json:"founder_address"`
	TotalAmount     int64        `json:"total_amount"`
	State           string       `json:"state"`
	Milestones      []*Milestone `json:"milestones"`
	CreatedAt       int64        `json:"created_at"`
	UpdatedAt       int64        `json:"updated_at"`
}

// NewContract creates a contract in DRAFT state. TotalAmount must equal the
// sum of every milestone's Amount, or an error is returned.
func NewContract(id, investor, founder string, milestones []*Milestone) (*Contract, error) {
	total := TotalAmount(milestones)
	if total <= 0 {
		return nil, fmt.Errorf("contract must have a positive total amount across milestones")
	}
	now := time.Now().Unix()
	return &Contract{
		ID:              id,
		InvestorAddress: investor,
		FounderAddress:  founder,
		TotalAmount:     total,
		State:           StateDraft,
		Milestones:      milestones,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// Advance validates and applies a lifecycle transition to the contract.
func (c *Contract) Advance(to string) error {
	if err := Transition(c.State, to); err != nil {
		return err
	}
	c.State = to
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// BuildFundingTx builds (unsigned) the on-chain LOCK transaction that funds
// this contract's escrow. Caller must sign it with the investor's wallet and
// submit it, then call Advance(StateFunded) once it's confirmed on-chain.
func (c *Contract) BuildFundingTx(investorNonce int64) (*blockchain.Transaction, error) {
	if c.State != StateSigned {
		return nil, fmt.Errorf("contract must be SIGNED before funding, currently %s", c.State)
	}
	return blockchain.NewTransaction(
		blockchain.TxLock,
		c.InvestorAddress,
		c.FounderAddress,
		c.TotalAmount,
		c.ID,
		"",
		investorNonce,
	), nil
}

// findMilestone returns the milestone with the given ID, or nil.
func (c *Contract) findMilestone(milestoneID string) *Milestone {
	for _, m := range c.Milestones {
		if m.ID == milestoneID {
			return m
		}
	}
	return nil
}

// SubmitMilestone marks a milestone as SUBMITTED and moves the contract into
// MILESTONE_REVIEW. Called when the founder reports a milestone complete.
func (c *Contract) SubmitMilestone(milestoneID string) error {
	m := c.findMilestone(milestoneID)
	if m == nil {
		return fmt.Errorf("milestone %s not found on contract %s", milestoneID, c.ID)
	}
	if m.Status != MilestonePending {
		return fmt.Errorf("milestone %s is not pending, currently %s", milestoneID, m.Status)
	}
	if err := c.Advance(StateMilestoneReview); err != nil {
		return err
	}
	m.Status = MilestoneSubmitted
	m.SubmittedAt = time.Now().Unix()
	return nil
}

// BuildMilestoneReleaseTx builds (unsigned) the on-chain RELEASE_MILESTONE
// transaction paying the founder for an approved milestone. authorityAddress
// is the platform escrow authority's own address - it is the entity that
// signs this transaction (see internal/api), not either contract party.
func (c *Contract) BuildMilestoneReleaseTx(milestoneID, authorityAddress string, releaseNonce int64) (*blockchain.Transaction, error) {
	m := c.findMilestone(milestoneID)
	if m == nil {
		return nil, fmt.Errorf("milestone %s not found on contract %s", milestoneID, c.ID)
	}
	if m.Status != MilestoneSubmitted {
		return nil, fmt.Errorf("milestone %s must be SUBMITTED before release, currently %s", milestoneID, m.Status)
	}
	return blockchain.NewTransaction(
		blockchain.TxReleaseMilestone,
		authorityAddress,
		c.FounderAddress,
		m.Amount,
		c.ID,
		milestoneID,
		releaseNonce,
	), nil
}

// ApproveMilestone marks a milestone APPROVED after its release transaction
// has been confirmed on-chain, and advances the contract - back to
// IN_PROGRESS if milestones remain, or to COMPLETED if this was the last one.
func (c *Contract) ApproveMilestone(milestoneID string) error {
	m := c.findMilestone(milestoneID)
	if m == nil {
		return fmt.Errorf("milestone %s not found on contract %s", milestoneID, c.ID)
	}
	m.Status = MilestoneApproved
	m.ApprovedAt = time.Now().Unix()

	if RemainingAmount(c.Milestones) == 0 {
		return c.Advance(StateCompleted)
	}
	return c.Advance(StateInProgress)
}

// DisputeMilestone marks a submitted milestone as disputed and moves the
// contract into DISPUTED, pending arbitration.
func (c *Contract) DisputeMilestone(milestoneID string) error {
	m := c.findMilestone(milestoneID)
	if m == nil {
		return fmt.Errorf("milestone %s not found on contract %s", milestoneID, c.ID)
	}
	if m.Status != MilestoneSubmitted {
		return fmt.Errorf("milestone %s must be SUBMITTED to dispute, currently %s", milestoneID, m.Status)
	}
	if err := c.Advance(StateDisputed); err != nil {
		return err
	}
	m.Status = MilestoneDisputed
	return nil
}

// BuildArbitrationTx builds (unsigned) the on-chain ARBITRATE transaction
// splitting or awarding the disputed milestone's escrowed amount to whichever
// party the arbitrator rules for. authorityAddress is the platform escrow
// authority's own address - the signer of this transaction.
func (c *Contract) BuildArbitrationTx(milestoneID, authorityAddress, awardTo string, amount int64, arbitratorNonce int64) (*blockchain.Transaction, error) {
	if c.State != StateDisputed {
		return nil, fmt.Errorf("contract must be DISPUTED to arbitrate, currently %s", c.State)
	}
	if awardTo != c.FounderAddress && awardTo != c.InvestorAddress {
		return nil, fmt.Errorf("arbitration must award either the founder or investor")
	}
	return blockchain.NewTransaction(
		blockchain.TxArbitrate,
		authorityAddress,
		awardTo,
		amount,
		c.ID,
		milestoneID,
		arbitratorNonce,
	), nil
}

// ResolveArbitration applies the outcome of arbitration once the ARBITRATE
// transaction is confirmed on-chain, resuming the contract if milestones
// remain or terminating it if the arbitrator ended the deal entirely.
func (c *Contract) ResolveArbitration(milestoneID string, terminate bool) error {
	if terminate {
		return c.Advance(StateTerminated)
	}
	m := c.findMilestone(milestoneID)
	if m != nil {
		m.Status = MilestoneApproved
		m.ApprovedAt = time.Now().Unix()
	}
	if RemainingAmount(c.Milestones) == 0 {
		return c.Advance(StateCompleted)
	}
	return c.Advance(StateInProgress)
}

// BuildRefundTx builds (unsigned) the on-chain REFUND transaction returning
// all remaining escrowed funds to the investor, used for mutual early
// termination. authorityAddress is the platform escrow authority's own
// address - the signer of this transaction.
func (c *Contract) BuildRefundTx(authorityAddress string, refundNonce int64) (*blockchain.Transaction, error) {
	remaining := RemainingAmount(c.Milestones)
	if remaining <= 0 {
		return nil, fmt.Errorf("no remaining escrow to refund on contract %s", c.ID)
	}
	return blockchain.NewTransaction(
		blockchain.TxRefund,
		authorityAddress,
		c.InvestorAddress,
		remaining,
		c.ID,
		"",
		refundNonce,
	), nil
}