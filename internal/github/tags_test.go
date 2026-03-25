package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/version"
)

// tagClientInterface defines the methods needed for tag operations
type tagClientInterface interface {
	getLatestTagRefs() ([]tagRef, error)
	checkTagExists(tagName string) (bool, error)
	createTagObject(tagName, message, commitSHA string) (string, error)
	createTagReference(tagName, tagSHA string) error
	getBranchSHA(branch string) (string, error)
}

// tagRef represents a git tag reference
type tagRef struct {
	Ref string `json:"ref"`
}

// mockTagClient implements tagClientInterface for testing
type mockTagClient struct {
	refs         []tagRef
	refsErr      error
	tagExists    map[string]bool
	tagExistsErr error
	tagObjectSHA string
	tagObjectErr error
	tagRefErr    error
	branchSHA    string
	branchErr    error
	calls        []string
}

func newMockTagClient() *mockTagClient {
	return &mockTagClient{
		tagExists: make(map[string]bool),
		calls:     []string{},
	}
}

func (m *mockTagClient) getLatestTagRefs() ([]tagRef, error) {
	m.calls = append(m.calls, "getLatestTagRefs")
	return m.refs, m.refsErr
}

func (m *mockTagClient) checkTagExists(tagName string) (bool, error) {
	m.calls = append(m.calls, "checkTagExists:"+tagName)
	if m.tagExistsErr != nil {
		return false, m.tagExistsErr
	}
	exists, ok := m.tagExists[tagName]
	if !ok {
		return false, nil
	}
	return exists, nil
}

func (m *mockTagClient) createTagObject(tagName, _, _ string) (string, error) {
	m.calls = append(m.calls, "createTagObject:"+tagName)
	if m.tagObjectErr != nil {
		return "", m.tagObjectErr
	}
	return m.tagObjectSHA, nil
}

func (m *mockTagClient) createTagReference(tagName, _ string) error {
	m.calls = append(m.calls, "createTagReference:"+tagName)
	return m.tagRefErr
}

func (m *mockTagClient) getBranchSHA(branch string) (string, error) {
	m.calls = append(m.calls, "getBranchSHA:"+branch)
	if m.branchErr != nil {
		return "", m.branchErr
	}
	return m.branchSHA, nil
}

// getLatestTagLogic implements the core logic for GetLatestTag using the interface
func getLatestTagLogic(client tagClientInterface) (string, error) {
	refs, err := client.getLatestTagRefs()
	if err != nil {
		// Check if it's a 404 (no tags exist)
		if isNotFoundError(err) {
			return defaultVersion, nil
		}
		return "", fmt.Errorf("fetching tags: %w", err)
	}

	if len(refs) == 0 {
		return defaultVersion, nil
	}

	// Extract version numbers from tag refs
	var versions []version.Version
	for _, ref := range refs {
		tagName := extractVersionFromRef(ref.Ref)
		v, err := version.Parse(tagName)
		if err != nil {
			// Skip non-semver tags
			continue
		}
		versions = append(versions, v)
	}

	if len(versions) == 0 {
		return defaultVersion, nil
	}

	// Find the highest version
	latest := versions[0]
	for _, v := range versions[1:] {
		if compareVersions(v, latest) > 0 {
			latest = v
		}
	}

	return latest.String(), nil
}

// tagExistsLogic implements the core logic for TagExists using the interface
func tagExistsLogic(client tagClientInterface, tagName string) (bool, error) {
	return client.checkTagExists(tagName)
}

// createTagLogic implements the core logic for CreateTag using the interface
func createTagLogic(client tagClientInterface, tagName, branch, message string) error {
	// Get the SHA of the latest commit on the branch
	commitSHA, err := client.getBranchSHA(branch)
	if err != nil {
		return fmt.Errorf("fetching branch %s: %w", branch, err)
	}

	// Create the tag object
	tagSHA, err := client.createTagObject(tagName, message, commitSHA)
	if err != nil {
		return fmt.Errorf("creating tag object: %w", err)
	}

	// Create the reference
	if err := client.createTagReference(tagName, tagSHA); err != nil {
		return fmt.Errorf("creating tag reference: %w", err)
	}

	return nil
}

// Helper functions
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return containsStr(err.Error(), "404")
}

func extractVersionFromRef(ref string) string {
	// refs/tags/v1.2.3 -> 1.2.3
	prefix := "refs/tags/"
	if len(ref) > len(prefix) && ref[:len(prefix)] == prefix {
		ref = ref[len(prefix):]
	}
	// Remove v prefix if present
	if len(ref) > 0 && ref[0] == 'v' {
		ref = ref[1:]
	}
	return ref
}

func compareVersions(a, b version.Version) int {
	if a.Major != b.Major {
		if a.Major > b.Major {
			return 1
		}
		return -1
	}
	if a.Minor != b.Minor {
		if a.Minor > b.Minor {
			return 1
		}
		return -1
	}
	if a.Patch != b.Patch {
		if a.Patch > b.Patch {
			return 1
		}
		return -1
	}
	return 0
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGetLatestTagLogic(t *testing.T) {
	tests := []struct {
		name        string
		refs        []tagRef
		refsErr     error
		expected    string
		expectError bool
	}{
		{
			name:        "no tags exist (404 error)",
			refsErr:     errors.New("404 Not Found"),
			expected:    "0.0.0",
			expectError: false,
		},
		{
			name:        "empty tags list",
			refs:        []tagRef{},
			expected:    "0.0.0",
			expectError: false,
		},
		{
			name:        "single tag",
			refs:        []tagRef{{Ref: "refs/tags/v1.0.0"}},
			expected:    "1.0.0",
			expectError: false,
		},
		{
			name: "multiple tags - returns highest version",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v2.0.0"},
				{Ref: "refs/tags/v1.5.0"},
			},
			expected:    "2.0.0",
			expectError: false,
		},
		{
			name: "tags without v prefix",
			refs: []tagRef{
				{Ref: "refs/tags/1.2.3"},
				{Ref: "refs/tags/2.0.0"},
			},
			expected:    "2.0.0",
			expectError: false,
		},
		{
			name: "mixed valid and invalid semver tags",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/invalid"},
				{Ref: "refs/tags/v2.0.0"},
			},
			expected:    "2.0.0",
			expectError: false,
		},
		{
			name: "all invalid semver tags",
			refs: []tagRef{
				{Ref: "refs/tags/invalid"},
				{Ref: "refs/tags/also-invalid"},
			},
			expected:    "0.0.0",
			expectError: false,
		},
		{
			name:        "network error",
			refsErr:     errors.New("network error"),
			expected:    "",
			expectError: true,
		},
		{
			name: "patch version sorting",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v1.0.5"},
				{Ref: "refs/tags/v1.0.10"},
			},
			expected:    "1.0.10",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMockTagClient()
			mc.refs = tt.refs
			mc.refsErr = tt.refsErr

			result, err := getLatestTagLogic(mc)

			if tt.expectError {
				if err == nil {
					t.Errorf("getLatestTagLogic() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("getLatestTagLogic() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("getLatestTagLogic() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTagExistsLogic(t *testing.T) {
	tests := []struct {
		name         string
		tagName      string
		tagExists    map[string]bool
		tagExistsErr error
		expected     bool
		expectError  bool
	}{
		{
			name:      "tag exists",
			tagName:   "v1.0.0",
			tagExists: map[string]bool{"v1.0.0": true},
			expected:  true,
		},
		{
			name:      "tag does not exist",
			tagName:   "v999.999.999",
			tagExists: map[string]bool{},
			expected:  false,
		},
		{
			name:         "error checking tag",
			tagName:      "v1.0.0",
			tagExistsErr: errors.New("network error"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMockTagClient()
			mc.tagExists = tt.tagExists
			mc.tagExistsErr = tt.tagExistsErr

			result, err := tagExistsLogic(mc, tt.tagName)

			if tt.expectError {
				if err == nil {
					t.Errorf("tagExistsLogic() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("tagExistsLogic() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("tagExistsLogic() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCreateTagLogic(t *testing.T) {
	tests := []struct {
		name         string
		tagName      string
		branch       string
		message      string
		branchSHA    string
		branchErr    error
		tagObjectSHA string
		tagObjectErr error
		tagRefErr    error
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "successful tag creation",
			tagName:      "v1.0.0",
			branch:       "master",
			message:      "Release v1.0.0",
			branchSHA:    "abc123",
			tagObjectSHA: "def456",
			expectError:  false,
		},
		{
			name:        "branch fetch fails",
			tagName:     "v1.0.0",
			branch:      "master",
			message:     "Release v1.0.0",
			branchErr:   errors.New("branch not found"),
			expectError: true,
			errorMsg:    "fetching branch",
		},
		{
			name:         "tag object creation fails",
			tagName:      "v1.0.0",
			branch:       "master",
			message:      "Release v1.0.0",
			branchSHA:    "abc123",
			tagObjectErr: errors.New("tag creation failed"),
			expectError:  true,
			errorMsg:     "creating tag object",
		},
		{
			name:         "tag reference creation fails",
			tagName:      "v1.0.0",
			branch:       "master",
			message:      "Release v1.0.0",
			branchSHA:    "abc123",
			tagObjectSHA: "def456",
			tagRefErr:    errors.New("ref creation failed"),
			expectError:  true,
			errorMsg:     "creating tag reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMockTagClient()
			mc.branchSHA = tt.branchSHA
			mc.branchErr = tt.branchErr
			mc.tagObjectSHA = tt.tagObjectSHA
			mc.tagObjectErr = tt.tagObjectErr
			mc.tagRefErr = tt.tagRefErr

			err := createTagLogic(mc, tt.tagName, tt.branch, tt.message)

			if tt.expectError {
				if err == nil {
					t.Errorf("createTagLogic() expected error, got nil")
					return
				}
				if tt.errorMsg != "" && !containsStr(err.Error(), tt.errorMsg) {
					t.Errorf("createTagLogic() error = %q, should contain %q", err.Error(), tt.errorMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("createTagLogic() unexpected error: %v", err)
			}
		})
	}
}

func TestExtractVersionFromRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{"with v prefix", "refs/tags/v1.2.3", "1.2.3"},
		{"without v prefix", "refs/tags/1.2.3", "1.2.3"},
		{"just version", "v1.2.3", "1.2.3"},
		{"plain version", "1.2.3", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromRef(tt.ref)
			if result != tt.expected {
				t.Errorf("extractVersionFromRef(%q) = %q, want %q", tt.ref, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		a        version.Version
		b        version.Version
		expected int
	}{
		{"equal versions", version.Version{Major: 1, Minor: 2, Patch: 3}, version.Version{Major: 1, Minor: 2, Patch: 3}, 0},
		{"major greater", version.Version{Major: 2, Minor: 0, Patch: 0}, version.Version{Major: 1, Minor: 9, Patch: 9}, 1},
		{"major lesser", version.Version{Major: 1, Minor: 9, Patch: 9}, version.Version{Major: 2, Minor: 0, Patch: 0}, -1},
		{"minor greater", version.Version{Major: 1, Minor: 2, Patch: 0}, version.Version{Major: 1, Minor: 1, Patch: 9}, 1},
		{"minor lesser", version.Version{Major: 1, Minor: 1, Patch: 9}, version.Version{Major: 1, Minor: 2, Patch: 0}, -1},
		{"patch greater", version.Version{Major: 1, Minor: 0, Patch: 2}, version.Version{Major: 1, Minor: 0, Patch: 1}, 1},
		{"patch lesser", version.Version{Major: 1, Minor: 0, Patch: 1}, version.Version{Major: 1, Minor: 0, Patch: 2}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("compareVersions(%v, %v) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestGetLatestTagSorting(t *testing.T) {
	tests := []struct {
		name     string
		refs     []tagRef
		expected string
	}{
		{
			name: "major version sorting",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v2.0.0"},
				{Ref: "refs/tags/v10.0.0"},
			},
			expected: "10.0.0",
		},
		{
			name: "minor version sorting",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v1.10.0"},
				{Ref: "refs/tags/v1.2.0"},
			},
			expected: "1.10.0",
		},
		{
			name: "patch version sorting",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v1.0.10"},
				{Ref: "refs/tags/v1.0.2"},
			},
			expected: "1.0.10",
		},
		{
			name: "complex mixed sorting",
			refs: []tagRef{
				{Ref: "refs/tags/v1.0.0"},
				{Ref: "refs/tags/v2.0.0"},
				{Ref: "refs/tags/v1.10.0"},
				{Ref: "refs/tags/v1.5.5"},
			},
			expected: "2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMockTagClient()
			mc.refs = tt.refs

			result, err := getLatestTagLogic(mc)
			if err != nil {
				t.Errorf("getLatestTagLogic() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("getLatestTagLogic() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test JSON parsing logic that would be used by the actual methods

func TestTagRefParsing(t *testing.T) {
	jsonData := `[
		{"ref": "refs/tags/v1.0.0"},
		{"ref": "refs/tags/v2.0.0"},
		{"ref": "refs/tags/v1.5.0"}
	]`

	var refs []tagRef
	if err := json.Unmarshal([]byte(jsonData), &refs); err != nil {
		t.Errorf("Failed to parse tag refs: %v", err)
	}

	if len(refs) != 3 {
		t.Errorf("Expected 3 refs, got %d", len(refs))
	}

	expected := []string{"refs/tags/v1.0.0", "refs/tags/v2.0.0", "refs/tags/v1.5.0"}
	for i, ref := range refs {
		if ref.Ref != expected[i] {
			t.Errorf("Expected ref %q, got %q", expected[i], ref.Ref)
		}
	}
}

func TestBranchRefParsing(t *testing.T) {
	jsonData := `{"object": {"sha": "abc123def456"}}`

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal([]byte(jsonData), &ref); err != nil {
		t.Errorf("Failed to parse branch ref: %v", err)
	}

	if ref.Object.SHA != "abc123def456" {
		t.Errorf("Expected SHA %q, got %q", "abc123def456", ref.Object.SHA)
	}
}

func TestTagResultParsing(t *testing.T) {
	jsonData := `{"sha": "def789abc123"}`

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Errorf("Failed to parse tag result: %v", err)
	}

	if result.SHA != "def789abc123" {
		t.Errorf("Expected SHA %q, got %q", "def789abc123", result.SHA)
	}
}
