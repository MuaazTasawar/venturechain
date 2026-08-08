package escrow

import "fmt"

// ContractState is one of the nine states in the contract lifecycle machine
// (BO-7): every investment contract moves through these in order, with
// IN_PROGRESS <-> MILESTONE_REVIEW looping once per milestone, and DISPUTED
// as the only state reachable from more than one place.
const (
	StateDraft            = "DRAFT"             // 1. created, not yet shown to counterparty
	StatePendingSignature = "PENDING_SIGNATURE" // 2. both parties reviewing terms
	StateSigned           = "SIGNED"            // 3. both signatures collected, awaiting funding
	StateFunded           = "FUNDED"            // 4. investor's funds locked in escrow (on-chain LOCK)
	StateInProgress       = "IN_PROGRESS"       // 5. founder executing, no milestone currently under review
	StateMilestoneReview  = "MILESTONE_REVIEW"  // 6. founder submitted a milestone, investor deciding
	StateDisputed         = "DISPUTED"          // 7. investor disputed a submission, awaiting arbitration
	StateCompleted        = "COMPLETED"         // 8. terminal - all milestones approved and released
	StateTerminated       = "TERMINATED"        // 9. terminal - contract ended early (refund/arbitration)
)

// validTransitions enumerates every legal ContractState -> ContractState
// move. Anything not listed here is rejected by Transition.
var validTransitions = map[string][]string{
	StateDraft:            {StatePendingSignature},
	StatePendingSignature: {StateSigned, StateDraft}, // Draft = terms sent back for revision
	StateSigned:           {StateFunded},
	StateFunded:           {StateInProgress},
	StateInProgress:       {StateMilestoneReview, StateTerminated}, // Terminated = mutual early exit
	StateMilestoneReview:  {StateInProgress, StateCompleted, StateDisputed},
	StateDisputed:         {StateInProgress, StateTerminated}, // arbitration resumes or ends the contract
	StateCompleted:        {},
	StateTerminated:       {},
}

// IsTerminal reports whether a state has no further legal transitions.
func IsTerminal(state string) bool {
	next, ok := validTransitions[state]
	return ok && len(next) == 0
}

// Transition validates a proposed state change and returns an error if it's
// not a legal move in the lifecycle machine. It does not mutate anything -
// callers apply the new state themselves after this passes.
func Transition(from, to string) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown contract state: %s", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("illegal transition: %s -> %s", from, to)
}