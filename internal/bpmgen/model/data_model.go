package model

type DataModel struct {
	Goal              string `json:"goal"`
	Draft             string `json:"draft"`
	Review            Review `json:"review"`
	Round             int    `json:"round"`
	MaxRounds         int    `json:"maxRounds"`
	Outcome           string `json:"outcome"`
	AITimeoutSeconds  int    `json:"aiTimeoutSeconds"`
	IterationDeadline string `json:"iterationDeadline"`
	EscalationReason  string `json:"escalationReason"`
	HumanApproved     bool   `json:"humanApproved"`
	HumanComment      string `json:"humanComment"`
}
type Review struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback"`
}
