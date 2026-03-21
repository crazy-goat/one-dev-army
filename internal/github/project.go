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

	out, err := c.ghNoRepo("project", "list", "--owner", owner, "--format", "json")
	if err != nil {
		return Project{}, fmt.Errorf("listing projects: %w", err)
	}
	var result struct {
		Projects []Project `json:"projects"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return Project{}, fmt.Errorf("parsing projects: %w", err)
	}
	projects := result.Projects

	for _, p := range projects {
		if p.Title == name {
			c.setupProject(p.Number)
			return p, nil
		}
	}

	out, err = c.ghNoRepo("project", "create", "--owner", owner, "--title", name, "--format", "json")
	if err != nil {
		return Project{}, fmt.Errorf("creating project %s: %w", name, err)
	}

	var created Project
	if err := parseJSON(out, &created); err != nil {
		return Project{}, err
	}

	// Link the new project to the repo and make it public.
	c.setupProject(created.Number)

	return created, nil
}

// setupProject links the project to the repo and sets visibility to public.
// Errors are ignored — these are best-effort (project works without them).
func (c *Client) setupProject(projectNumber int) {
	owner := strings.Split(c.Repo, "/")[0]
	num := fmt.Sprintf("%d", projectNumber)
	c.ghNoRepo("project", "link", num, "--owner", owner, "--repo", c.Repo)
	c.ghNoRepo("project", "edit", num, "--owner", owner, "--visibility", "PUBLIC")
}

type fieldOption struct {
	Name string `json:"name"`
}

type projectField struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Options []fieldOption `json:"options"`
}

type fieldList struct {
	Fields []projectField `json:"fields"`
}

// EnsureProjectColumns ensures the project's Status field contains all
// required columns in the correct order. Overwrites options if they differ.
func (c *Client) EnsureProjectColumns(projectID string, projectNumber int) error {
	owner := strings.Split(c.Repo, "/")[0]

	out, err := c.ghNoRepo("project", "field-list", fmt.Sprintf("%d", projectNumber),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("listing project fields: %w", err)
	}

	var fl fieldList
	if err := json.Unmarshal(out, &fl); err != nil {
		return fmt.Errorf("parsing project fields: %w", err)
	}

	// Find the Status field.
	var statusField *projectField
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

	// Check if options match exactly (same names, same order).
	if optionsMatch(statusField.Options, ProjectColumns) {
		return nil
	}

	// Options differ — overwrite with the canonical set.
	return c.updateStatusFieldOptions(statusField.ID, ProjectColumns)
}

// optionsMatch returns true if the existing options match the expected list exactly.
func optionsMatch(existing []fieldOption, expected []string) bool {
	if len(existing) != len(expected) {
		return false
	}
	for i, o := range existing {
		if o.Name != expected[i] {
			return false
		}
	}
	return true
}

// createStatusField creates a new Status SINGLE_SELECT field with all columns.
func (c *Client) createStatusField(projectID string, projectNumber int, owner string, options []string) error {
	_, err := c.ghNoRepo(
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

// columnColors maps column names to GitHub project colors.
var columnColors = map[string]string{
	"Backlog":     "GRAY",
	"In Progress": "BLUE",
	"Review":      "YELLOW",
	"Merging":     "PURPLE",
	"Done":        "GREEN",
	"Blocked":     "RED",
}

// updateStatusFieldOptions replaces all options on the Status field in one GraphQL call.
func (c *Client) updateStatusFieldOptions(fieldID string, options []string) error {
	// Build inline options list for the mutation.
	var opts []string
	for _, name := range options {
		color := columnColors[name]
		if color == "" {
			color = "GRAY"
		}
		opts = append(opts, fmt.Sprintf(`{name: %q, color: %s, description: ""}`, name, color))
	}
	query := fmt.Sprintf(`mutation {
		updateProjectV2Field(input: {
			fieldId: %q
			singleSelectOptions: [%s]
		}) {
			projectV2Field {
				... on ProjectV2SingleSelectField { id }
			}
		}
	}`, fieldID, strings.Join(opts, ", "))

	_, err := c.ghNoRepo("api", "graphql", "-f", "query="+query)
	if err != nil {
		return fmt.Errorf("updating Status field options: %w", err)
	}
	return nil
}
