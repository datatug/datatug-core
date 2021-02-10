package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// RecordsetRequestParams is a set of common request parameters
type QueryRequestParams struct {
	Project string `json:"project"`
	Query   string `json:"query"`
}

// Validate returns error if not valid
func (v QueryRequestParams) Validate() error {
	if v.Project == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.Query == "" {
		return validation.NewErrRequestIsMissingRequiredField("query")
	}
	return nil
}

// GetQueries returns queries
func GetQueries(projectID, folder string) ([]models.QueryDef, error) {
	return store.Current.LoadQueries(projectID, folder)
}

// CreateQuery creates a new query
func CreateQuery(params QueryRequestParams, query models.QueryDef) error {
	if err := params.Validate(); err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	return store.Current.CreateQuery(params.Project, query)
}

// UpdateQuery updates existing query
func UpdateQuery(params QueryRequestParams, query models.QueryDef) error {
	if err := params.Validate(); err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	return store.Current.UpdateQuery(params.Project, query)
}

// DeleteQuery deletes query
func DeleteQuery(projectID string, queryID string) error {
	params := QueryRequestParams{Project: projectID, Query: queryID}
	if err := params.Validate(); err != nil {
		return err
	}
	return store.Current.DeleteQuery(params.Project, params.Query)
}

// DeleteQuery deletes query
func GetQuery(params QueryRequestParams) (query models.QueryDef, err error) {
	if err = params.Validate(); err != nil {
		return query, err
	}
	return store.Current.LoadQuery(params.Project, params.Query)
}
