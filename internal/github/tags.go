package github

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/version"
)

const defaultVersion = "0.0.0"

// Tag represents a git tag
type Tag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// GetLatestTag returns the latest semantic version tag from the repository
// If no tags exist, returns defaultVersion and nil error
func (c *Client) GetLatestTag() (string, error) {
	// Fetch all tags using gh CLI
	out, err := c.ghNoRepo("api", "repos/"+c.Repo+"/git/refs/tags")
	if err != nil {
		// Check if it's a 404 (no tags exist)
		if strings.Contains(err.Error(), "404") {
			return defaultVersion, nil
		}
		return "", fmt.Errorf("fetching tags: %w", err)
	}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(out, &refs); err != nil {
		return "", fmt.Errorf("parsing tags: %w", err)
	}

	if len(refs) == 0 {
		return defaultVersion, nil
	}

	// Extract version numbers from tag refs (refs/tags/v1.2.3 -> 1.2.3)
	var versions []version.Version
	for _, ref := range refs {
		tagName := strings.TrimPrefix(ref.Ref, "refs/tags/")
		tagName = strings.TrimPrefix(tagName, "v")

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

	// Sort versions in descending order
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Major != versions[j].Major {
			return versions[i].Major > versions[j].Major
		}
		if versions[i].Minor != versions[j].Minor {
			return versions[i].Minor > versions[j].Minor
		}
		return versions[i].Patch > versions[j].Patch
	})

	return versions[0].String(), nil
}

// TagExists checks if a tag already exists in the repository
func (c *Client) TagExists(tagName string) (bool, error) {
	_, err := c.ghNoRepo("api", "repos/"+c.Repo+"/git/refs/tags/"+tagName)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("checking tag existence: %w", err)
	}
	return true, nil
}

// CreateTag creates a new annotated tag on the specified branch
func (c *Client) CreateTag(tagName, branch, message string) error {
	// First, get the SHA of the latest commit on the branch
	out, err := c.ghNoRepo("api", "repos/"+c.Repo+"/git/ref/heads/"+branch)
	if err != nil {
		return fmt.Errorf("fetching branch %s: %w", branch, err)
	}

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(out, &ref); err != nil {
		return fmt.Errorf("parsing branch ref: %w", err)
	}

	commitSHA := ref.Object.SHA

	// Create the tag object
	tagObj, err := c.ghNoRepo("api", "repos/"+c.Repo+"/git/tags",
		"-f", "tag="+tagName,
		"-f", "message="+message,
		"-f", "object="+commitSHA,
		"-f", "type=commit")
	if err != nil {
		return fmt.Errorf("creating tag object: %w", err)
	}

	var tagResult struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(tagObj, &tagResult); err != nil {
		return fmt.Errorf("parsing tag result: %w", err)
	}

	// Create the reference (refs/tags/tagName)
	_, err = c.ghNoRepo("api", "repos/"+c.Repo+"/git/refs",
		"-f", "ref=refs/tags/"+tagName,
		"-f", "sha="+tagResult.SHA)
	if err != nil {
		return fmt.Errorf("creating tag reference: %w", err)
	}

	return nil
}
