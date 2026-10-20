package datatug

import (
	"fmt"
	"strings"
)

// TimelineChartSpec maps one complete admitted held result into milestone lanes.
// timeColumn contains observed RFC3339 instants or null for unattempted events;
// sequenceColumn is an integer sequence within a lane/attempt. Clock domain and
// precision are per-row evidence, never inferred from labels or browser time.
// evidenceRefColumn contains typed artifact references resolved under current ACL,
// never a URL or protected cached row. Missing spans must remain unavailable.
type TimelineChartSpec struct {
	LaneColumn        string `json:"laneColumn"`
	EventColumn       string `json:"eventColumn"`
	SequenceColumn    string `json:"sequenceColumn"`
	TimeColumn        string `json:"timeColumn"`
	ClockDomainColumn string `json:"clockDomainColumn"`
	PrecisionColumn   string `json:"precisionColumn"`
	StatusColumn      string `json:"statusColumn"`
	EvidenceRefColumn string `json:"evidenceRefColumn"`
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
	return distinctEvidenceColumns(s.LaneColumn, s.EventColumn, s.SequenceColumn, s.TimeColumn, s.ClockDomainColumn, s.PrecisionColumn, s.StatusColumn, s.EvidenceRefColumn)
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
		if v.Timeline == nil || v.Heatmap != nil {
			return fmt.Errorf("timeline requires only a timeline mapping")
		}
		return v.Timeline.Validate()
	case "heatmap":
		if v.Heatmap == nil || v.Timeline != nil {
			return fmt.Errorf("heatmap requires only a heatmap mapping")
		}
		return v.Heatmap.Validate()
	}
	return fmt.Errorf("unknown evidence chart")
}
