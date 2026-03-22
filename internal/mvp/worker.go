package mvp

import (
	"context"
	"regexp"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type Worker struct {
	id      int
	cfg     *config.Config
	oc      *opencode.Client
	gh      *github.Client
	wtMgr   *git.WorktreeManager
	baseDir string
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, wtMgr *git.WorktreeManager) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		wtMgr:   wtMgr,
		baseDir: wtMgr.WorktreesDir(),
	}
}

func (w *Worker) Process(ctx context.Context, task *Task) error {
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(title string) string {
	s := strings.ToLower(title)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func extractText(msg *opencode.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range msg.Parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}
