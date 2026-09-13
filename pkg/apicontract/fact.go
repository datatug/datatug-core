package apicontract

import "github.com/datatug/datatug-core/pkg/investigation"

// Fact is an alias of the one canonical Investigation Context fact model.
type Fact = investigation.Fact

// ProjectScope is persisted fact provenance; request Scope additionally
// carries the current security-context staleness token and remains separate.
type ProjectScope = investigation.ProjectScope

const (
	FactOriginSelection = investigation.FactOriginSelection
	FactOriginContext   = investigation.FactOriginContext
	FactOriginManual    = investigation.FactOriginManual

	FactRoleAffected       = investigation.FactRoleAffected
	FactRoleHealthyControl = investigation.FactRoleHealthyControl
	FactRoleSuspected      = investigation.FactRoleSuspected
	FactRoleExcluded       = investigation.FactRoleExcluded
	FactRoleRecovered      = investigation.FactRoleRecovered

	FactMappingDeclared = investigation.FactMappingDeclared
	FactMappingInferred = investigation.FactMappingInferred

	FactConditionEqual              = investigation.FactConditionEqual
	FactConditionNotEqual           = investigation.FactConditionNotEqual
	FactConditionGreaterThan        = investigation.FactConditionGreaterThan
	FactConditionGreaterThanOrEqual = investigation.FactConditionGreaterThanOrEqual
	FactConditionLessThan           = investigation.FactConditionLessThan
	FactConditionLessThanOrEqual    = investigation.FactConditionLessThanOrEqual
)

// InvestigationContext is the shared ordered context carried by an
// investigation and attached to an incident's canonical layer.
type InvestigationContext = investigation.Context
