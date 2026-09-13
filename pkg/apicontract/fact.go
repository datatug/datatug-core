package apicontract

import "github.com/datatug/datatug-core/pkg/investigation"

// Fact is an alias of the one canonical Investigation Context fact model.
type Fact = investigation.Fact

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
)

// InvestigationContext is the shared ordered context carried by an
// investigation and attached to an incident's canonical layer.
type InvestigationContext = investigation.Context
