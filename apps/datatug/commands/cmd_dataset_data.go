package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
)

// datasetCommand defines parameters for test command
type datasetDataCommand struct {
	datasetBaseCommand
	File   string `long:"file" required:"true"`
	Format string `long:"format"`
	Indent string `long:"indent" description:"Pass a digit to specify number of spaces (default=1). Special value: 'TAB'."`
}

// Execute executes test command
func (v *datasetDataCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" command
	ctx := context.Background()
	recordset, err := v.store.GetProjectStore(v.projectID).Recordsets().Recordset(v.Dataset).LoadRecordsetData(ctx, v.File)
	if err != nil {
		return err
	}
	if v.Format == "" {
		v.Format = "yaml"
	}

	var encoder Encoder

	switch strings.ToUpper(v.Format) {
	case "YAML":
		yamlEncoder := yaml.NewEncoder(os.Stdout)
		if len(v.Indent) == 1 {
			indent, err := strconv.Atoi(v.Indent)
			if err != nil {
				yamlEncoder.SetIndent(indent)
			}
		} else if strings.ToUpper(v.Indent) == "TAB" {
			yamlEncoder.SetIndent(4)
		}
		encoder = yamlEncoder
		defer func() {
			closeErr := yamlEncoder.Close()
			if err != nil {
				err = closeErr
			}
		}()
	case "JSON":
		jsonEncoder := json.NewEncoder(os.Stdout)
		switch strings.ToUpper(v.Indent) {
		case "":
			v.Indent = " "
		case "TAB":
			v.Indent = "\t"
		default:
			indent, err := strconv.Atoi(v.Indent)
			if err != nil {
				v.Indent = strings.Repeat(" ", indent)
			}
		}
		jsonEncoder.SetIndent("", v.Indent)
		encoder = jsonEncoder
	case "GRID":
		return showRecordsetInGrid(*recordset)
	default:
		return fmt.Errorf("unknown or unsopported format (expected JSON, YAML, GRID): %v", v.Format)
	}
	return writeRows(*recordset, encoder)
}

// Encoder defines interface to encode
type Encoder interface {
	Encode(v interface{}) error
}

func writeRows(recordset models.Recordset, encoder Encoder) error {
	rows := make([]map[string]interface{}, 0, len(recordset.Rows))

	for _, values := range recordset.Rows {
		row := make(map[string]interface{})
		for i, col := range recordset.Columns {
			row[col.Name] = values[i]
		}
		rows = append(rows, row)
	}
	return encoder.Encode(rows)
}
