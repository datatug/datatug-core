// Package policyscope limits a local access-policy rule to an environment, an
// instance, or both (backstage spec/features/perimeter, REQ:local-rule-selectors
// and permission model §4.1).
//
// A rule in a DALgo access document may carry two DataTug keys that DALgo
// itself does not know:
//
//	environment: production            # an environment kind
//	instance: factory-e-prod/factory   # <environment id>/<catalog id>
//
// Project turns such a document into the plain DALgo document for one read:
// a rule whose selectors all match the instance the read opens keeps its
// place with the keys removed; any other selector rule is removed, as if the
// document did not contain it. DALgo precedence and deny by default are left
// to DALgo. A document with no selector is returned as the same bytes. When
// the read's instance (or, for an environment selector, its kind) is not
// known, Project refuses with ErrTargetUnknown and the caller must deny.
package policyscope

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Rule keys this package owns.
const (
	KeyEnvironment = "environment"
	KeyInstance    = "instance"
)

// ErrTargetUnknown is returned for a document that uses a selector when the
// read's instance, or the environment kind a selector needs, is not known.
// Every such document denies that read.
var ErrTargetUnknown = errors.New("policyscope: the instance this read opens is not known, so a policy that selects instances or environments denies it")

// ErrInvalidSelector marks a document whose selectors cannot be applied. The
// whole document is refused, never applied without them.
var ErrInvalidSelector = errors.New("policyscope: invalid rule selector")

// Target is the instance one read opens: one catalog in one environment
// (decision 0008), and the kind stored on that environment.
type Target struct {
	// Environment is the environment id, for example factory-e-prod.
	Environment string
	// Catalog is the catalog id inside the environment, for example factory.
	Catalog string
	// Kind is the environment's kind; empty or unknown means not known.
	Kind datatug.EnvironmentKind
}

// Instance is the target's instance id, <environment>/<catalog>, or "" when
// either part is not known.
func (t Target) Instance() string {
	if t.Environment == "" || t.Catalog == "" {
		return ""
	}
	return t.Environment + "/" + t.Catalog
}

func (t Target) kindKnown() bool {
	return t.Kind != "" && t.Kind != datatug.EnvironmentKindUnknown
}

// Selector is the selectors one rule names. The zero Selector names none.
type Selector struct {
	Environment datatug.EnvironmentKind
	Instance    string
}

// IsZero reports whether the selector names nothing.
func (s Selector) IsZero() bool { return s.Environment == "" && s.Instance == "" }

// Validate refuses an environment that is not a known kind and an instance
// not written <environment>/<catalog>.
func (s Selector) Validate() error {
	if s.Environment != "" {
		if err := s.Environment.Validate(); err != nil || s.Environment == datatug.EnvironmentKindUnknown {
			return fmt.Errorf("%w: environment %q is not an environment kind (%s)", ErrInvalidSelector, string(s.Environment), kindList())
		}
	}
	if s.Instance != "" {
		environment, catalog, ok := strings.Cut(s.Instance, "/")
		if !ok || !plainID(environment) || !plainID(catalog) {
			return fmt.Errorf("%w: instance %q must be <environment>/<catalog>, for example factory-e-prod/factory", ErrInvalidSelector, s.Instance)
		}
	}
	return nil
}

// Matches reports whether t satisfies every selector s names. It returns
// ErrTargetUnknown when t lacks a fact a selector needs.
func (s Selector) Matches(t Target) (bool, error) {
	if s.Environment != "" && !t.kindKnown() || s.Instance != "" && t.Instance() == "" {
		return false, ErrTargetUnknown
	}
	return (s.Environment == "" || s.Environment == t.Kind) && (s.Instance == "" || s.Instance == t.Instance()), nil
}

func plainID(id string) bool {
	return id != "" && !strings.ContainsFunc(id, func(r rune) bool {
		return r == '/' || unicode.IsSpace(r) || unicode.IsControl(r)
	})
}

func kindList() string {
	var kinds []string
	for _, kind := range datatug.EnvironmentKinds {
		if kind != datatug.EnvironmentKindUnknown {
			kinds = append(kinds, string(kind))
		}
	}
	return strings.Join(kinds, ", ")
}
