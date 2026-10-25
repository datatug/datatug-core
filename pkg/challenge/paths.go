package challenge

import (
	"fmt"
	"strconv"
	"strings"
)

// AssetPath is the project-relative path of a published challenge revision.
func AssetPath(challengeID string, revision int) (string, error) {
	if !validID(challengeID) {
		return "", refuse(CodeInvalidManifest, "challengeId", "must be a lowercase slug of at most %d characters", maxChallengeIDLen)
	}
	if revision < 1 || revision > maxRevision {
		return "", refuse(CodeInvalidManifest, "revision", "must be between 1 and %d", maxRevision)
	}
	return fmt.Sprintf("challenges/%s/%d.json", challengeID, revision), nil
}

// GuidedRouteSuffix is the project-scoped route suffix that opens the existing
// query page in guided mode: /project/<projectRef>/challenge/<id>/v/<revision>.
// Core deliberately does not parse the project reference; the caller owns its
// exact spelling and pins the returned revision.
func GuidedRouteSuffix(challengeID string, revision int) (string, error) {
	if !validID(challengeID) || revision < 1 || revision > maxRevision {
		return "", refuse(CodeInvalidManifest, "challenge", "invalid challenge id or revision")
	}
	return "challenge/" + challengeID + "/v/" + strconv.Itoa(revision), nil
}

// FixtureAssetPath is the conventional project-relative path of a fixture
// inside the challenge asset directory. Fixture identity is always the pinned
// SHA-256, not this path.
func FixtureAssetPath(challengeID, fixtureID string) (string, error) {
	if !validID(challengeID) || !validID(fixtureID) {
		return "", refuse(CodeInvalidManifest, "fixture", "invalid challenge or fixture id")
	}
	return "challenges/" + challengeID + "/fixtures/" + fixtureID + ".db", nil
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > maxChallengeIDLen || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func validQueryRef(id string) bool {
	if id == "" || len(id) > 512 {
		return false
	}
	for _, segment := range strings.Split(id, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > 128 {
			return false
		}
		for _, r := range segment {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
				return false
			}
		}
	}
	return true
}

func validSQLName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if r < ' ' || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

func boundedText(value string, min, max int) bool {
	return len(value) >= min && len(value) <= max
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
