package datatug

type ApiService struct {
	ProjectItem
}

func (v ApiService) Validate() error {
	if err := v.ValidateWithOptions(true); err != nil {
		return err
	}
	return nil
}

type ApiEndpoint struct {
	ProjectItem

	QueryType string `json:"queryType" firestore:"queryType"` // REST, RPC, GraphQL
	Method    string `json:"method" firestore:"method"`
	UrlSchema string `json:"urlSchema" firestore:"urlSchema"`
	UrlHost   string `json:"urlHost" firestore:"urlHost"`
	UrlPath   string `json:"urlPath" firestore:"urlPath"`

	FavoritesCount int `json:"favoritesCount" firestore:"favoritesCount"`
	VotesScore     int `json:"votesScore" firestore:"votesScore"`
	VotesUp        int `json:"votesUp" firestore:"votesUp"`
	VotesDown      int `json:"votesDown" firestore:"votesDown"`
}

func (v ApiEndpoint) Validate() error {
	if err := v.ValidateWithOptions(false); err != nil {
		return err
	}
	return nil
}
