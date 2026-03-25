package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a semantic version (major.minor.patch)
type Version struct {
	Major int
	Minor int
	Patch int
}

// String returns the version as a string (e.g., "1.2.3")
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Parse parses a version string into a Version struct
// Returns error if the string is not in valid semver format
func Parse(version string) (Version, error) {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version format: %s (expected major.minor.patch)", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %s", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %s", parts[2])
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// BumpMajor returns a new Version with major bumped, minor and patch reset to 0
func (v Version) BumpMajor() Version {
	return Version{Major: v.Major + 1, Minor: 0, Patch: 0}
}

// BumpMinor returns a new Version with minor bumped, patch reset to 0
func (v Version) BumpMinor() Version {
	return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
}

// BumpPatch returns a new Version with patch bumped
func (v Version) BumpPatch() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}
