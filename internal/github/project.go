package github

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

var ProjectColumns = []string{
	"Blocked",
	"Backlog",
	"Plan",
	"Code",
	"AI Review",
	"Approve",
	"Done",
	"Failed",
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
			c.ProjectID = p.ID
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

	c.ProjectID = created.ID
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
	ID   string `json:"id"`
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

	c.StatusFieldID = statusField.ID

	if optionsMatch(statusField.Options, ProjectColumns) {
		c.StatusOptionIDs = make(map[string]string, len(statusField.Options))
		for _, opt := range statusField.Options {
			c.StatusOptionIDs[opt.Name] = opt.ID
		}
		log.Printf("[GitHub] Status field ID: %s, options: %v", statusField.ID, c.StatusOptionIDs)
		return nil
	}

	// Options differ — overwrite with the canonical set, then re-fetch to get new IDs.
	if err := c.updateStatusFieldOptions(statusField.ID, ProjectColumns); err != nil {
		return err
	}
	return c.refreshStatusOptionIDs(projectNumber, owner)
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
	"Blocked":   "RED",
	"Backlog":   "GRAY",
	"Plan":      "YELLOW",
	"Code":      "BLUE",
	"AI Review": "YELLOW",
	"Approve":   "PURPLE",
	"Done":      "GREEN",
	"Failed":    "RED",
}

// ProjectItem represents an item in a GitHub Project with its status
type ProjectItem struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// GetProjectItemsByStatus fetches all items from a project and groups them by status
func (c *Client) GetProjectItemsByStatus(projectNumber int) (map[string][]ProjectItem, error) {
	owner := strings.Split(c.Repo, "/")[0]

	// Fetch project items with their fields
	out, err := c.ghNoRepo("project", "item-list", fmt.Sprintf("%d", projectNumber),
		"--owner", owner, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("listing project items: %w", err)
	}

	var result struct {
		Items []struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
			Title  string `json:"title"`
			Fields []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"fields"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing project items: %w", err)
	}

	// Group items by status
	itemsByStatus := make(map[string][]ProjectItem)
	for _, col := range ProjectColumns {
		itemsByStatus[col] = []ProjectItem{}
	}

	for _, item := range result.Items {
		status := "Backlog"
		for _, field := range item.Fields {
			if field.Name == "Status" && field.Value != "" {
				status = field.Value
				break
			}
		}

		projectItem := ProjectItem{
			Number: item.Number,
			Title:  item.Title,
			Status: status,
		}
		itemsByStatus[status] = append(itemsByStatus[status], projectItem)
	}

	var counts []string
	for _, col := range ProjectColumns {
		counts = append(counts, fmt.Sprintf("%s:%d", col, len(itemsByStatus[col])))
	}
	log.Printf("[GitHub] Project items: %s", strings.Join(counts, " | "))

	return itemsByStatus, nil
}

// MoveItemToColumn moves an issue to a specific column in the project board.
// It first ensures the issue is added to the project, then sets its Status field.
func (c *Client) MoveItemToColumn(projectNumber int, issueNumber int, column string) error {
	owner := strings.Split(c.Repo, "/")[0]
	num := fmt.Sprintf("%d", projectNumber)

	repo := strings.Split(c.Repo, "/")
	if len(repo) != 2 {
		return fmt.Errorf("invalid repo format: %s", c.Repo)
	}

	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", c.Repo, issueNumber)

	out, err := c.ghNoRepo("project", "item-add", num,
		"--owner", owner,
		"--url", issueURL,
		"--format", "json")
	if err != nil {
		log.Printf("[GitHub] item-add may have failed (item might already exist): %v", err)
	}

	var addResult struct {
		ID string `json:"id"`
	}
	if out != nil {
		json.Unmarshal(out, &addResult)
	}

	itemID := addResult.ID
	if itemID == "" {
		items, err := c.GetProjectItemsByStatus(projectNumber)
		if err != nil {
			return fmt.Errorf("getting project items: %w", err)
		}
		for _, colItems := range items {
			for _, item := range colItems {
				if item.Number == issueNumber {
					itemID = item.ID
					break
				}
			}
			if itemID != "" {
				break
			}
		}
	}

	if itemID == "" {
		return fmt.Errorf("could not find project item for issue #%d", issueNumber)
	}

	projectID := c.ProjectID
	if projectID == "" {
		return fmt.Errorf("project node ID not set — call EnsureProject first")
	}

	fieldID := c.StatusFieldID
	if fieldID == "" {
		fieldID = "Status"
	}

	optionID := column
	if c.StatusOptionIDs != nil {
		if id, ok := c.StatusOptionIDs[column]; ok {
			optionID = id
		}
	}

	_, err = c.ghNoRepo("project", "item-edit",
		"--id", itemID,
		"--project-id", projectID,
		"--field-id", fieldID,
		"--single-select-option-id", optionID)
	if err != nil {
		return fmt.Errorf("setting item status to %q: %w", column, err)
	}

	log.Printf("[GitHub] Moved #%d to %q in project %d", issueNumber, column, projectNumber)
	return nil
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

func (c *Client) refreshStatusOptionIDs(projectNumber int, owner string) error {
	out, err := c.ghNoRepo("project", "field-list", fmt.Sprintf("%d", projectNumber),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("refreshing status options: %w", err)
	}

	var fl fieldList
	if err := json.Unmarshal(out, &fl); err != nil {
		return fmt.Errorf("parsing refreshed fields: %w", err)
	}

	for _, f := range fl.Fields {
		if f.Name == "Status" {
			c.StatusOptionIDs = make(map[string]string, len(f.Options))
			for _, opt := range f.Options {
				c.StatusOptionIDs[opt.Name] = opt.ID
			}
			log.Printf("[GitHub] Refreshed Status options: %v", c.StatusOptionIDs)
			return nil
		}
	}

	return fmt.Errorf("Status field not found after update")
}
