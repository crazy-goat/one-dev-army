package github

import (
	"encoding/json"
	"fmt"
	"strings"
)

var ProjectColumns = []string{
	"Backlog",
	"In Progress",
	"Review",
	"Merging",
	"Done",
	"Blocked",
}

type Project struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// EnsureProject ensures a GitHub Project with the given name exists for the
// repo owner. Returns the project node ID and number. Creates the project if
// it does not exist.
func (c *Client) EnsureProject(name string) (Project, error) {
	owner := strings.Split(c.Repo, "/")[0]

	var projects []Project
	err := c.ghJSON(&projects, "project", "list", "--owner", owner, "--format", "json")
	if err != nil {
		return Project{}, fmt.Errorf("listing projects: %w", err)
	}

	for _, p := range projects {
		if p.Title == name {
			return p, nil
		}
	}

	out, err := c.gh("project", "create", "--owner", owner, "--title", name, "--format", "json")
	if err != nil {
		return Project{}, fmt.Errorf("creating project %s: %w", name, err)
	}

	var created Project
	if err := parseJSON(out, &created); err != nil {
		return Project{}, err
	}
	return created, nil
}

// EnsureProjectColumns ensures the project's Status field contains all
// required columns. Missing options are added via GraphQL API.
func (c *Client) EnsureProjectColumns(projectID string, projectNumber int) error {
	owner := strings.Split(c.Repo, "/")[0]

	// Fetch existing fields via gh project field-list.
	type fieldOption struct {
		Name string `json:"name"`
	}
	type field struct {
		ID      string        `json:"id"`
		Name    string        `json:"name"`
		Options []fieldOption `json:"options"`
	}
	type fieldList struct {
		Fields []field `json:"fields"`
	}

	out, err := c.gh("project", "field-list", fmt.Sprintf("%d", projectNumber),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("listing project fields: %w", err)
	}

	var fl fieldList
	if err := json.Unmarshal(out, &fl); err != nil {
		return fmt.Errorf("parsing project fields: %w", err)
	}

	// Find the Status field.
	var statusField *field
	for i := range fl.Fields {
		if fl.Fields[i].Name == "Status" {
			statusField = &fl.Fields[i]
			break
		}
	}

	if statusField == nil {
		// No Status field — create it with all required options.
		return c.createStatusField(projectID, projectNumber, owner, ProjectColumns)
	}

	// Determine which options are missing.
	existing := make(map[string]bool, len(statusField.Options))
	for _, o := range statusField.Options {
		existing[o.Name] = true
	}

	var missing []string
	for _, col := range ProjectColumns {
		if !existing[col] {
			missing = append(missing, col)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Add missing options via GraphQL.
	for _, name := range missing {
		if err := c.addStatusFieldOption(projectID, statusField.ID, name); err != nil {
			return fmt.Errorf("adding status option %q: %w", name, err)
		}
	}

	return nil
}

// createStatusField creates a new Status SINGLE_SELECT field with all columns.
func (c *Client) createStatusField(projectID string, projectNumber int, owner string, options []string) error {
	_, err := c.gh(
		"project", "field-create", fmt.Sprintf("%d", projectNumber),
		"--owner", owner,
		"--name", "Status",
		"--data-type", "SINGLE_SELECT",
		"--single-select-options", strings.Join(options, ","),
	)
	if err != nil {
		return fmt.Errorf("creating Status field: %w", err)
	}
	return nil
}

// addStatusFieldOption adds a single option to an existing SINGLE_SELECT field
// using the GitHub GraphQL API.
func (c *Client) addStatusFieldOption(projectID, fieldID, optionName string) error {
	query := `mutation($projectId: ID!, $fieldId: ID!, $name: String!) {
		updateProjectV2Field(input: {
			projectId: $projectId
			fieldId: $fieldId
			singleSelectField: {
				options: [{name: $name}]
			}
		}) {
			projectV2Field {
				... on ProjectV2SingleSelectField {
					id
				}
			}
		}
	}`

	_, err := c.gh("api", "graphql",
		"-f", fmt.Sprintf("query=%s", query),
		"-f", fmt.Sprintf("projectId=%s", projectID),
		"-f", fmt.Sprintf("fieldId=%s", fieldID),
		"-f", fmt.Sprintf("name=%s", optionName),
	)
	if err != nil {
		return fmt.Errorf("graphql addStatusFieldOption: %w", err)
	}
	return nil
}
