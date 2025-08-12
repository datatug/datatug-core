package gauth

// ServiceAccountsUpdatedMsg is emitted by fbauth UIs to notify parent models that
// the list of service accounts has changed.
// Payload is the full, current list.
type ServiceAccountsUpdatedMsg []ServiceAccountDbo

const defaultFilepathDir = "datatug"
