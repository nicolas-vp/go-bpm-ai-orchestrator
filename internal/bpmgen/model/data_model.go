package model

type DataModel struct {
	Goal      string `json:"goal"`
	Draft     string `json:"draft"`
	Review    Review `json:"review"`
	Round     int    `json:"round"`
	MaxRounds int    `json:"maxRounds"`
	Outcome   string `json:"outcome"`
}
type Review struct {
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback"`
}
