package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

// TimelineChartSpec maps one complete admitted held result into milestone lanes.
// timeColumn contains observed RFC3339 instants or null for unattempted events;
// sequenceColumn is an integer sequence within a lane/attempt. Clock domain and
// precision are per-row evidence, never inferred from labels or browser time.
// evidenceRefColumn contains typed artifact references resolved under current ACL,
// never a URL or protected cached row. Missing spans must remain unavailable.
type TimelineChartSpec struct {
	TimeMode          string `json:"timeMode,omitempty" yaml:"timeMode,omitempty"`
	AnchorColumn      string `json:"anchorColumn,omitempty" yaml:"anchorColumn,omitempty"`
	LaneColumn        string `json:"laneColumn" yaml:"laneColumn"`
	EventColumn       string `json:"eventColumn" yaml:"eventColumn"`
	SequenceColumn    string `json:"sequenceColumn" yaml:"sequenceColumn"`
	TimeColumn        string `json:"timeColumn" yaml:"timeColumn"`
	ClockDomainColumn string `json:"clockDomainColumn" yaml:"clockDomainColumn"`
	PrecisionColumn   string `json:"precisionColumn" yaml:"precisionColumn"`
	StatusColumn      string `json:"statusColumn" yaml:"statusColumn"`
	EvidenceRefColumn string `json:"evidenceRefColumn" yaml:"evidenceRefColumn"`
}

// HeatmapChartSpec maps long-form rows to a categorical matrix. The held status
// values are exactly pass, fail or unknown; duplicate cells are rejected, never
// averaged or silently replaced. Null/unavailable is unknown, never pass/zero.
type HeatmapChartSpec struct {
	RowColumn         string `json:"rowColumn"`
	ColumnColumn      string `json:"columnColumn"`
	StatusColumn      string `json:"statusColumn"`
	EvidenceRefColumn string `json:"evidenceRefColumn"`
}

func distinctEvidenceColumns(columns ...string) error {
	seen := map[string]bool{}
	for _, column := range columns {
		if column == "" || strings.TrimSpace(column) != column {
			return fmt.Errorf("evidence chart column names must be nonempty and canonical")
		}
		if seen[column] {
			return fmt.Errorf("evidence chart columns must differ: %s", column)
		}
		seen[column] = true
	}
	return nil
}
func (s TimelineChartSpec) Validate() error {
	columns := []string{s.LaneColumn, s.EventColumn, s.SequenceColumn, s.TimeColumn, s.ClockDomainColumn, s.PrecisionColumn, s.StatusColumn, s.EvidenceRefColumn}
	switch s.TimeMode {
	case "", "absolute":
		if s.AnchorColumn != "" {
			return fmt.Errorf("anchorColumn requires elapsed timeMode")
		}
	case "elapsed":
		columns = append(columns, s.AnchorColumn)
	default:
		return fmt.Errorf("unsupported timeline timeMode")
	}
	return distinctEvidenceColumns(columns...)
}
func (s HeatmapChartSpec) Validate() error {
	return distinctEvidenceColumns(s.RowColumn, s.ColumnColumn, s.StatusColumn, s.EvidenceRefColumn)
}
func (v ChartSpec) validateEvidenceChart() error {
	if v.CategoryColumn != "" || v.ValueColumn != "" || v.Series != nil || v.PeriodColumn != "" || v.Unit != "" || v.Grain != "" || v.OnDuplicate != "" || v.Mode != "" || v.TopN != 0 {
		return fmt.Errorf("evidence charts reject bar/race fields")
	}
	if v.SelectionID == "" || strings.TrimSpace(v.SelectionID) != v.SelectionID {
		return fmt.Errorf("evidence chart requires a canonical selectionId")
	}
	switch v.Type {
	case "timeline":
		if v.Timeline == nil || v.Heatmap != nil || v.Timing != nil {
			return fmt.Errorf("timeline requires only a timeline mapping")
		}
		return v.Timeline.Validate()
	case "timing-bars":
		if v.Timing == nil || v.Timeline != nil || v.Heatmap != nil {
			return fmt.Errorf("timing-bars requires only a timing mapping")
		}
		return v.Timing.Validate()
	case "heatmap":
		if v.Heatmap == nil || v.Timeline != nil || v.Timing != nil {
			return fmt.Errorf("heatmap requires only a heatmap mapping")
		}
		return v.Heatmap.Validate()
	}
	return fmt.Errorf("unknown evidence chart")
}

// TimingChartSpec persists column mappings, not durations or protected results.
// Observed endpoint identity, clock/precision compatibility and admitted evidence
// must be checked against the held result by the provider/consumer.
type TimingChartSpec struct {
	RowKeyColumn           string `json:"rowKeyColumn"`
	LabelColumn            string `json:"labelColumn"`
	StartTimeColumn        string `json:"startTimeColumn"`
	EndTimeColumn          string `json:"endTimeColumn"`
	StartClockDomainColumn string `json:"startClockDomainColumn"`
	EndClockDomainColumn   string `json:"endClockDomainColumn"`
	StartPrecisionColumn   string `json:"startPrecisionColumn"`
	EndPrecisionColumn     string `json:"endPrecisionColumn"`
	StartEvidenceRefColumn string `json:"startEvidenceRefColumn"`
	EndEvidenceRefColumn   string `json:"endEvidenceRefColumn"`
	StartIdentityColumn    string `json:"startIdentityColumn"`
	EndIdentityColumn      string `json:"endIdentityColumn"`
}

func (s TimingChartSpec) Validate() error {
	// Shared clock and precision mappings are legitimate; other roles must differ.
	columns := []string{s.RowKeyColumn, s.LabelColumn, s.StartTimeColumn, s.EndTimeColumn, s.StartClockDomainColumn, s.StartPrecisionColumn, s.StartEvidenceRefColumn, s.EndEvidenceRefColumn, s.StartIdentityColumn, s.EndIdentityColumn}
	if s.EndClockDomainColumn != s.StartClockDomainColumn {
		columns = append(columns, s.EndClockDomainColumn)
	}
	if s.EndPrecisionColumn != s.StartPrecisionColumn {
		columns = append(columns, s.EndPrecisionColumn)
	}
	return distinctEvidenceColumns(columns...)
}

// UnmarshalJSON enforces the closed canonical mapping wire even in nested boards.
func (s *TimingChartSpec) UnmarshalJSON(data []byte) error {
	type wire TimingChartSpec
	if err := jsonstrict.CheckNoDuplicateKeysForSchema(data, reflect.TypeOf(wire{})); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"rowKeyColumn": true, "labelColumn": true, "startTimeColumn": true, "endTimeColumn": true, "startClockDomainColumn": true, "endClockDomainColumn": true, "startPrecisionColumn": true, "endPrecisionColumn": true, "startEvidenceRefColumn": true, "endEvidenceRefColumn": true, "startIdentityColumn": true, "endIdentityColumn": true}
	for name, value := range fields {
		if !allowed[name] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("unsupported or null timing mapping %s", name)
		}
	}
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = TimingChartSpec(decoded)
	return nil
}
