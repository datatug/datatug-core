package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"strings"
)

// SelectRequest holds request data
type SelectRequest struct {
	Project     string
	Environment string
	Database    string
	From        string
	SQL         string
	Where       string
	Limit       int
	Columns     []string
	Parameters  []models.Parameter
}

// Validate return error if not valid
func (v SelectRequest) Validate() error {
	if v.From == "" && v.SQL == "" {
		return validation.NewErrRequestIsMissingRequiredField("from OR sql")
	}
	if v.SQL != "" {
		if v.From != "" {
			return validation.NewErrBadRequestFieldValue("from", "can't be supplied when 'sql' parameter passed")
		}
		if v.Where != "" {
			return validation.NewErrBadRequestFieldValue("where", "can't be supplied when 'sql' parameter passed")
		}
		if len(v.Columns) > 0 {
			return validation.NewErrBadRequestFieldValue("cols", "can't be supplied when 'sql' parameter passed")
		}
	}
	if v.Project == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.Environment == "" {
		return validation.NewErrRequestIsMissingRequiredField("environment")
	}
	if v.Database == "" {
		return validation.NewErrRequestIsMissingRequiredField("database")
	}
	if v.Where != "" && !strings.Contains(v.Where, ":") {
		return validation.NewErrBadRequestFieldValue("where", "should have : char to separate field from value")
	}
	if v.Limit < -1 {
		return validation.NewErrBadRequestFieldValue("limit", "should be >= -1")
	}
	return nil
}

// ExecuteSelect executes select command
func ExecuteSelect(selectRequest SelectRequest) (response execute.Response, err error) {
	if err = selectRequest.Validate(); err != nil {
		return
	}
	command := execute.RequestCommand{
		Env: selectRequest.Environment,
		DB:  selectRequest.Database,
	}
	if selectRequest.SQL == "" {
		if selectRequest.Limit >= 0 {
			command.Text += fmt.Sprintf("select top %v ", selectRequest.Limit)
		} else {
			command.Text += "select "
		}
		if len(selectRequest.Columns) > 0 {
			command.Text += strings.Join(selectRequest.Columns, ", ")
		} else {
			command.Text += "*"
		}
		command.Text += " from " + selectRequest.From
		if selectRequest.Where != "" {
			conditions := strings.Split(selectRequest.Where, ";")
			for i, condition := range conditions {
				if i == 0 {
					command.Text += " where "
				} else {
					command.Text += " and "
				}
				where := strings.Split(condition, ":")
				command.Text += fmt.Sprintf("%v = '%v'", where[0], where[1])
			}
		}
	} else {
		command.Text = selectRequest.SQL
	}
	command.Parameters = selectRequest.Parameters[:]
	request := execute.Request{
		Project: selectRequest.Project,
		Commands: []execute.RequestCommand{
			command,
		},
	}
	response, err = ExecuteCommands(request)
	if err != nil {
		err = fmt.Errorf("failed to execute select command: %w\n%v", err, command.Text)
	}
	return
}
