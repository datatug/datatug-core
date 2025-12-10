package datatug

// ChangesetDef defines a set of changes to be applied
type ChangesetDef struct {
	ProjectItem
	Datasets []ChangesetRefToDataset `json:"datasets"`
}

// ChangesetRefToDataset defines a reference to a dataset
type ChangesetRefToDataset struct {
	ID       string `json:"id"`
	Required bool   `json:"required"`
}

// Changeset holds a set of data changes to be applied
type Changeset struct {
	Status   string       `json:"status"`
	Datasets []DatasetDef `json:"datasets"`
}
