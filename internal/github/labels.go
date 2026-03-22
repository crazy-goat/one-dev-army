package github

import (
	"fmt"
	"sync"
)

type Label struct {
	Name  string
	Color string
}

var RequiredLabels = []Label{
	{Name: "sprint", Color: "0E8A16"},
	{Name: "insight", Color: "D93F0B"},
	{Name: "in-progress", Color: "FBCA04"},
	{Name: "failed", Color: "D93F0B"},
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
	{Name: "priority:high", Color: "B60205"},
	{Name: "priority:medium", Color: "FBCA04"},
	{Name: "priority:low", Color: "0E8A16"},
	{Name: "epic", Color: "5319E7"},
}

func (c *Client) EnsureLabels() error {
	existing, _ := c.listLabels()
	existingSet := make(map[string]bool, len(existing))
	for _, name := range existing {
		existingSet[name] = true
	}

	var missing []Label
	for _, l := range RequiredLabels {
		if !existingSet[l.Name] {
			missing = append(missing, l)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(missing))

	for _, l := range missing {
		wg.Add(1)
		go func(l Label) {
			defer wg.Done()
			_, err := c.gh("label", "create", l.Name, "--color", l.Color, "--force")
			if err != nil {
				errs <- fmt.Errorf("creating label %s: %w", l.Name, err)
			}
		}(l)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		return err
	}
	return nil
}

func (c *Client) listLabels() ([]string, error) {
	var labels []struct {
		Name string `json:"name"`
	}
	err := c.ghJSON(&labels, "label", "list", "--json", "name", "--limit", "200")
	if err != nil {
		return nil, err
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names, nil
}
