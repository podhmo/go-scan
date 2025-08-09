package enum

type Status int

const (
	Pending Status = 0
	Active  Status = 1
	Failed  Status = 2
)

type UntypedStatus int

const (
	UntypedPending UntypedStatus = iota
	UntypedActive
	UntypedFailed
)

type StringStatus string

const (
	StringStatusOK StringStatus = "ok"
	StringStatusNG StringStatus = "ng"
)
