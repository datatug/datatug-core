package models

type ApiService struct {
	ProjectItem
}

type ApiEndpoint struct {
	ProjectItem
	QueryType    string   `json:"queryType" firestore:"queryType"`       // http, GraphQL
	ContentTypes []string `json:"contentTypes" firestore:"contentTypes"` // application/json, text
	Method       string   `json:"method" firestore:"method"`
	Schema       string   `json:"schema" firestore:"schema"`
	UrlHost      string   `json:"urlHost" firestore:"urlHost"`
	UrlPath      string   `json:"urlPath" firestore:"urlPath"`

	FavoritesCount int `json:"favoritesCount" firestore:"favoritesCount"`
	VotesScore     int `json:"votesScore" firestore:"votesScore"`
	VotesUp        int `json:"votesUp" firestore:"votesUp"`
	VotesDown      int `json:"votesDown" firestore:"votesDown"`
}
