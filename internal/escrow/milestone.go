package escrow

// MilestoneStatus tracks an individual milestone within a contract,
// independent of the overall ContractState.
const (
	MilestonePending  = "PENDING"   // not yet started
	MilestoneSubmitted = "SUBMITTED" // founder submitted proof of completion
	MilestoneApproved = "APPROVED"  // investor approved, funds released
	MilestoneDisputed = "DISPUTED"  // investor disputed the submission
)

// Milestone represents one funded deliverable within a Contract. Amount is
// the portion of the total escrow released when this milestone is approved.
type Milestone struct {
	ID          string `json:"id"`
	ContractID  string `json:"contract_id"`
	Title       string `json:"title"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
	SubmittedAt int64  `json:"submitted_at,omitempty"`
	ApprovedAt  int64  `json:"approved_at,omitempty"`
}

// NewMilestone creates a milestone in PENDING status.
func NewMilestone(id, contractID, title string, amount int64) *Milestone {
	return &Milestone{
		ID:         id,
		ContractID: contractID,
		Title:      title,
		Amount:     amount,
		Status:     MilestonePending,
	}
}

// RemainingAmount sums the Amount of every milestone not yet APPROVED,
// used to validate that a contract's total escrow matches its milestone
// breakdown when it's funded.
func RemainingAmount(milestones []*Milestone) int64 {
	var total int64
	for _, m := range milestones {
		if m.Status != MilestoneApproved {
			total += m.Amount
		}
	}
	return total
}

// TotalAmount sums the Amount of every milestone regardless of status,
// used to validate the contract's declared TotalAmount at creation time.
func TotalAmount(milestones []*Milestone) int64 {
	var total int64
	for _, m := range milestones {
		total += m.Amount
	}
	return total
}