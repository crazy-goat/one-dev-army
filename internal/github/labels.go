package github

import "fmt"

type Label struct {
	Name  string
	Color string
}

var RequiredLabels = []Label{
	{Name: "sprint", Color: "0E8A16"},
	{Name: "insight", Color: "D93F0B"},
	{Name: "size:S", Color: "C2E0C6"},
	{Name: "size:M", Color: "BFDADC"},
	{Name: "size:L", Color: "BFD4F2"},
	{Name: "size:XL", Color: "D4C5F9"},
	{Name: "stage:analysis", Color: "FBCA04"},
	{Name: "stage:planning", Color: "FBCA04"},
	{Name: "stage:plan-review", Color: "FBCA04"},
	{Name: "stage:coding", Color: "1D76DB"},
	{Name: "stage:testing", Color: "1D76DB"},
	{Name: "stage:code-review", Color: "1D76DB"},
	{Name: "stage:needs-user", Color: "B60205"},
	{Name: "stage:cancelled", Color: "EEEEEE"},
}

func (c *Client) EnsureLabels() error {
	for _, l := range RequiredLabels {
		_, err := c.gh("label", "create", l.Name, "--color", l.Color, "--force")
		if err != nil {
			return fmt.Errorf("creating label %s: %w", l.Name, err)
		}
	}
	return nil
}
