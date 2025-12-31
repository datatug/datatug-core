package datatug

type RepoRootFile struct {

	// List of paths to DataTug projects
	Projects []string `json:"projects,omitempty" yaml:"projects,omitempty"`
}
