package models

// ChangeType defines what kind of change performed or to be performed
type ChangeType int

//goland:noinspection GoUnusedConst
const (
	ChangeTypeUnchanged ChangeType = iota
	ChangeTypeAdded
	ChangeTypeAltered
	ChangeTypeDeleted
)
