package mvp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/plan"
	"github.com/crazy-goat/one-dev-army/internal/prompts"
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

var ErrAlreadyDone = errors.New("ticket already done")

type Worker struct {
	id           int
	cfg          atomic.Pointer[config.Config]
	oc           *opencode.Client
	gh           *github.Client
	brMgr        *git.BranchManager
	store        *db.Store
	repoDir      string
	orchestrator *Orchestrator
	router       *llm.Router
	decisionCh   chan UserDecision // receives approve/decline from dashboard
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, brMgr *git.BranchManager, store *db.Store, orchestrator *Orchestrator, router *llm.Router) *Worker {
	w := &Worker{
		id:           id,
		oc:           oc,
		gh:           gh,
		brMgr:        brMgr,
		store:        store,
		repoDir:      brMgr.RepoDir(),
		orchestrator: orchestrator,
		router:       router,
		decisionCh:   make(chan UserDecision, 1),
	}
	w.cfg.Store(cfg)
	return w
}

// Processes the event synchronously so that GitHub labels, cache, ledger,
// and WebSocket are updated immediately — not deferred until after Process().
func (w *Worker) reportStageComplete(stage string, status EventStatus, output string) {
	if w.orchestrator == nil || w.orchestrator.currentTask == nil {
		return
	}

	event := WorkerEvent{
		IssueNumber: w.orchestrator.currentTask.Issue.Number,
		Stage:       stage,
		Status:      status,
		Output:      output,
	}

	log.Printf("[Worker] Reporting completion of stage %s for issue #%d", stage, event.IssueNumber)
	w.orchestrator.HandleWorkerEvent(event)
}

var stepOrder = []string{"technical-planning", "implement", "code-review", "create-pr", "check-pipeline", "awaiting-approval", "merge"}

func stepIndex(name string) int {
	for i, s := range stepOrder {
		if s == name {
			return i
		}
	}
	return -1
}

func (w *Worker) Process(ctx context.Context, task *Task) error {
	log.Printf("[Worker %d] ▶ START processing #%d: %s", w.id, task.Issue.Number, task.Issue.Title)
	start := time.Now()
	task.StartTime = start

	resumeFrom := 0
	if w.store != nil {
		lastDone, _ := w.store.GetLastCompletedStep(task.Issue.Number)
		if lastDone != "" {
			idx := stepIndex(lastDone)
			if idx >= 0 {
				resumeFrom = idx + 1
				log.Printf("[Worker %d] Resuming from step %d (%s completed previously)", w.id, resumeFrom, lastDone)
			}
		}
	}

	task.Status = StatusAnalyzing

	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	log.Printf("[Worker %d] Creating branch %q", w.id, branch)
	if err := w.brMgr.CreateBranch(branch); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating branch: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating branch: %v", w.id, err)
		return task.Result.Error
	}
	task.Branch = branch
	task.Worktree = w.repoDir
	log.Printf("[Worker %d] Branch %s ready, working in %s", w.id, branch, w.repoDir)

	// Create artifact directory for this ticket
	var artifactDir string
	var err error
	artifactDir, err = w.createArtifactDir(task.Issue.Number)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating artifact directory: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating artifact directory: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] Artifact directory ready: %s", w.id, artifactDir)

	// Deferred cleanup: delete branch if task did not complete successfully.
	// On successful merge, gh pr merge --delete-branch handles branch deletion.
	defer func() {
		if task.Branch != "" && task.Status != StatusDone {
			log.Printf("[Worker %d] Cleaning up branch %q (task not done)", w.id, task.Branch)
			if err := w.brMgr.RemoveBranch(task.Branch); err != nil {
				log.Printf("[Worker %d] Warning: failed to remove branch %q: %v", w.id, task.Branch, err)
			}
		}
	}()

	var analysis, implPlan, prURL string
	_ = artifactDir

	if resumeFrom <= 0 {
		log.Printf("[Worker %d] [1/7] Technical planning for #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		stepLogger, slErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "technical-planning")
		if slErr != nil {
			log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, slErr)
		}
		if stepLogger != nil {
			_ = stepLogger.Start()
			defer stepLogger.Close()
		}
		analysis, implPlan, err = w.technicalPlanning(ctx, task, stepLogger)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("technical planning: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED technical planning: %v", w.id, err)
			if stepLogger != nil {
				_ = stepLogger.End(false, err.Error())
			}
			return task.Result.Error
		}
		log.Printf("[Worker %d] [1/7] Technical planning done (%s, analysis=%d chars, plan=%d chars)", w.id, time.Since(stepStart).Round(time.Second), len(analysis), len(implPlan))
		if stepLogger != nil {
			_ = stepLogger.End(true, fmt.Sprintf("analysis=%d chars, plan=%d chars", len(analysis), len(implPlan)))
		}
		w.reportStageComplete("analysis", EventSuccess, "technical planning completed")
	} else {
		log.Printf("[Worker %d] [1/7] Skipping technical-planning (completed previously)", w.id)
		if w.store != nil {
			// Try to get combined response from new step name
			response, _ := w.store.GetStepResponse(task.Issue.Number, "technical-planning")
			if response != "" {
				_, implPlan = parseTechnicalPlanningResponse(response)
			} else {
				// Fallback: try to get from old step names for backward compatibility
				_, _ = w.store.GetStepResponse(task.Issue.Number, "analyze")
				implPlan, _ = w.store.GetStepResponse(task.Issue.Number, "plan")
			}
		}
	}

	if resumeFrom <= 1 {
		task.Status = StatusCoding
		log.Printf("[Worker %d] [2/7] Implementing #%d (includes tests)...", w.id, task.Issue.Number)
		stepStart := time.Now()
		stepLogger, slErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "implement")
		if slErr != nil {
			log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, slErr)
		}
		if stepLogger != nil {
			_ = stepLogger.Start()
			defer stepLogger.Close()
		}
		if err := w.implement(ctx, task, implPlan, stepLogger); err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("implementing: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED implementing: %v", w.id, err)
			if stepLogger != nil {
				_ = stepLogger.End(false, err.Error())
			}
			return task.Result.Error
		}
		log.Printf("[Worker %d] [2/7] Implementation done (%s)", w.id, time.Since(stepStart).Round(time.Second))
		if stepLogger != nil {
			_ = stepLogger.End(true, "")
		}
		w.reportStageComplete("coding", EventSuccess, "implementation completed")
	} else {
		log.Printf("[Worker %d] [2/7] Skipping implement (completed previously)", w.id)
	}

	if resumeFrom <= 2 {
		task.Status = StatusReviewing
		log.Printf("[Worker %d] [3/7] Code review #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		stepLogger, slErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "code-review")
		if slErr != nil {
			log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, slErr)
		}
		if stepLogger != nil {
			_ = stepLogger.Start()
			defer stepLogger.Close()
		}
		approved, review, crErr := w.codeReview(ctx, task, "", stepLogger)
		if crErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("code review: %w", crErr)}
			log.Printf("[Worker %d] ✗ FAILED code review: %v", w.id, crErr)
			if stepLogger != nil {
				_ = stepLogger.End(false, crErr.Error())
			}
			return task.Result.Error
		}
		log.Printf("[Worker %d] [3/7] Code review done (%s, approved=%v)", w.id, time.Since(stepStart).Round(time.Second), approved)

		if !approved {
			log.Printf("[Worker %d] Code review not approved, fixing...", w.id)
			task.Status = StatusCoding
			w.reportStageComplete("coding", EventInProgress, "fixing from AI review")
			stepStart = time.Now()
			fixLogger, fixErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "fix-from-review")
			if fixErr != nil {
				log.Printf("[Worker %d] Warning: failed to create fix logger: %v", w.id, fixErr)
			}
			if fixLogger != nil {
				_ = fixLogger.Start()
				defer fixLogger.Close()
			}
			if fixErr := w.fixFromReview(ctx, task, review, fixLogger); fixErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("fixing from review: %w", fixErr)}
				log.Printf("[Worker %d] ✗ FAILED fixing from review: %v", w.id, fixErr)
				if fixLogger != nil {
					_ = fixLogger.End(false, fixErr.Error())
				}
				return task.Result.Error
			}
			if fixLogger != nil {
				_ = fixLogger.End(true, "")
			}
			log.Printf("[Worker %d] Fix from review done (%s), pushing...", w.id, time.Since(stepStart).Round(time.Second))
			if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("pushing fixes: %w", pushErr)}
				log.Printf("[Worker %d] ✗ FAILED pushing fixes: %v", w.id, pushErr)
				return task.Result.Error
			}
			log.Printf("[Worker %d] Re-running code review after fixes...", w.id)
			stepStart = time.Now()
			approved, _, crErr = w.codeReview(ctx, task, "", nil)
			if crErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("code review after fixes: %w", crErr)}
				log.Printf("[Worker %d] ✗ FAILED code review after fixes: %v", w.id, crErr)
				return task.Result.Error
			}
			log.Printf("[Worker %d] Code review after fixes done (%s, approved=%v)", w.id, time.Since(stepStart).Round(time.Second), approved)
			if !approved {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: errors.New("code review not approved after fixes")}
				log.Printf("[Worker %d] ✗ FAILED: code review not approved after fixes", w.id)
				return task.Result.Error
			}
		}
		if stepLogger != nil {
			_ = stepLogger.End(true, fmt.Sprintf("approved=%v", approved))
		}
		w.reportStageComplete("code-review", EventSuccess, "code review approved")
	} else {
		log.Printf("[Worker %d] [3/7] Skipping code-review (completed previously)", w.id)
	}

	if resumeFrom <= 3 {
		task.Status = StatusCreatingPR
		log.Printf("[Worker %d] [4/7] Creating PR for #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		stepLogger, slErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "create-pr")
		if slErr != nil {
			log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, slErr)
		}
		if stepLogger != nil {
			_ = stepLogger.Start()
			defer stepLogger.Close()
		}
		prURL, err = w.createPR(ctx, task, stepLogger)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED creating PR: %v", w.id, err)
			if stepLogger != nil {
				_ = stepLogger.End(false, err.Error())
			}
			return task.Result.Error
		}
		log.Printf("[Worker %d] [4/7] PR created: %s (%s)", w.id, prURL, time.Since(stepStart).Round(time.Second))
		if stepLogger != nil {
			_ = stepLogger.End(true, prURL)
		}
		w.reportStageComplete("create-pr", EventSuccess, "PR created: "+prURL)
	} else {
		log.Printf("[Worker %d] [4/7] Skipping create-pr (completed previously)", w.id)
		if w.store != nil {
			prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
		}
	}

	// === CHECK PIPELINE (with retry back to coding) ===
	cfg := w.cfg.Load()
	maxPipelineRetries := cfg.Pipeline.MaxRetries
	if maxPipelineRetries <= 0 {
		maxPipelineRetries = 5
	}
	pipelineAttempt := 0
	for {
		if resumeFrom <= 4 {
			task.Status = StatusCheckingPipeline
			log.Printf("[Worker %d] [5/7] Checking CI pipeline for #%d (attempt %d/%d)...", w.id, task.Issue.Number, pipelineAttempt+1, maxPipelineRetries)
			stepStart := time.Now()
			stepLogger, slErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "check-pipeline")
			if slErr != nil {
				log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, slErr)
			}
			if stepLogger != nil {
				_ = stepLogger.Start()
				defer stepLogger.Close()
			}
			if err := w.checkPipeline(ctx, task, stepLogger); err != nil {
				pipelineAttempt++
				if pipelineAttempt >= maxPipelineRetries {
					task.Status = StatusFailed
					task.Result = &TaskResult{Error: fmt.Errorf("pipeline check: %w (after %d attempts)", err, pipelineAttempt)}
					log.Printf("[Worker %d] ✗ FAILED pipeline check after %d attempts: %v", w.id, pipelineAttempt, err)
					if stepLogger != nil {
						_ = stepLogger.End(false, fmt.Sprintf("failed after %d attempts: %v", pipelineAttempt, err))
					}
					w.reportStageComplete("check-pipeline", EventFailed, err.Error())
					return task.Result.Error
				}
				log.Printf("[Worker %d] Pipeline failed (attempt %d/%d), retrying from coding...", w.id, pipelineAttempt, maxPipelineRetries)
				if stepLogger != nil {
					stepLogger.Logf("Pipeline failed (attempt %d/%d), retrying from coding", pipelineAttempt, maxPipelineRetries)
				}
				w.reportStageComplete("check-pipeline", EventFailed, fmt.Sprintf("attempt %d/%d failed, retrying coding", pipelineAttempt, maxPipelineRetries))

				task.Status = StatusCoding
				w.reportStageComplete("coding", EventInProgress, fmt.Sprintf("fixing pipeline failure (attempt %d)", pipelineAttempt))
				fixLogger, fixErr := worker.NewStepLogger(artifactDir, task.Issue.Number, fmt.Sprintf("fix-pipeline-attempt-%d", pipelineAttempt))
				if fixErr != nil {
					log.Printf("[Worker %d] Warning: failed to create fix logger: %v", w.id, fixErr)
				}
				if fixLogger != nil {
					_ = fixLogger.Start()
					defer fixLogger.Close()
				}
				if fixErr := w.implement(ctx, task, implPlan, fixLogger); fixErr != nil {
					task.Status = StatusFailed
					task.Result = &TaskResult{Error: fmt.Errorf("fixing from pipeline failure: %w", fixErr)}
					log.Printf("[Worker %d] ✗ FAILED fixing from pipeline failure: %v", w.id, fixErr)
					if fixLogger != nil {
						_ = fixLogger.End(false, fixErr.Error())
					}
					return task.Result.Error
				}
				if fixLogger != nil {
					_ = fixLogger.End(true, "")
				}
				if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
					task.Status = StatusFailed
					task.Result = &TaskResult{Error: fmt.Errorf("pushing pipeline fixes: %w", pushErr)}
					log.Printf("[Worker %d] ✗ FAILED pushing pipeline fixes: %v", w.id, pushErr)
					return task.Result.Error
				}
				resumeFrom = 4
				continue
			}
			log.Printf("[Worker %d] [5/7] Pipeline checks passed (%s)", w.id, time.Since(stepStart).Round(time.Second))
			if stepLogger != nil {
				_ = stepLogger.End(true, "all checks passed")
			}
			w.reportStageComplete("check-pipeline", EventSuccess, "all CI checks passed")
		} else {
			log.Printf("[Worker %d] [5/7] Skipping check-pipeline (completed previously)", w.id)
		}
		break
	}

	// If resuming from awaiting-approval or later, recover prURL from store
	if resumeFrom >= 5 && prURL == "" && w.store != nil {
		prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
	}

	// === APPROVAL + MERGE LOOP ===
	// Worker stays alive until ticket reaches terminal state.
	// User approve → merge → done. User decline → fix → re-review → wait again.
	for {
		task.Status = StatusAwaitingApproval
		log.Printf("[Worker %d] [6/7] Awaiting user approval for #%d (PR: %s)", w.id, task.Issue.Number, prURL)
		approvalLogger, alErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "awaiting-approval")
		if alErr != nil {
			log.Printf("[Worker %d] Warning: failed to create approval logger: %v", w.id, alErr)
		}
		if approvalLogger != nil {
			_ = approvalLogger.Start()
			defer approvalLogger.Close()
			approvalLogger.Logf("PR URL: %s", prURL)
		}
		w.reportStageComplete("awaiting-approval", EventInProgress, "PR created, awaiting approval: "+prURL)

		var decision UserDecision
		if w.cfg.Load().YoloMode {
			log.Printf("[Worker %d] YOLO mode enabled - auto-approving #%d", w.id, task.Issue.Number)
			if approvalLogger != nil {
				approvalLogger.Logf("YOLO mode enabled - auto-approving")
			}
			decision = UserDecision{Action: "approve"}
		} else {
			select {
			case decision = <-w.decisionCh:
				log.Printf("[Worker %d] Received decision for #%d: %s", w.id, task.Issue.Number, decision.Action)
				if approvalLogger != nil {
					approvalLogger.Logf("Received decision: %s", decision.Action)
					if decision.Reason != "" {
						approvalLogger.Logf("Decision reason: %s", decision.Reason)
					}
				}
			case <-ctx.Done():
				if approvalLogger != nil {
					approvalLogger.Logf("Context canceled: %v", ctx.Err())
					_ = approvalLogger.End(false, ctx.Err().Error())
				}
				return ctx.Err()
			}
		}

		if decision.Action == "approve" {
			task.Status = StatusMerging
			log.Printf("[Worker %d] [7/7] Merging PR for #%d (branch: %s)", w.id, task.Issue.Number, task.Branch)
			if approvalLogger != nil {
				_ = approvalLogger.End(true, "user approved")
			}
			w.reportStageComplete("awaiting-approval", EventSuccess, "user approved")

			mergeLogger, mlErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "merge")
			if mlErr != nil {
				log.Printf("[Worker %d] Warning: failed to create merge logger: %v", w.id, mlErr)
			}
			if mergeLogger != nil {
				_ = mergeLogger.Start()
				defer mergeLogger.Close()
				mergeLogger.Logf("Merging branch: %s", task.Branch)
			}

			if err := w.gh.MergePR(task.Branch); err != nil {
				log.Printf("[Worker %d] ✗ Merge failed for #%d: %v", w.id, task.Issue.Number, err)

				if closeErr := w.gh.ClosePR(task.Branch); closeErr != nil {
					log.Printf("[Worker %d] Error closing PR for #%d: %v", w.id, task.Issue.Number, closeErr)
				}

				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("merge failed: %w", err)}

				comment := "Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: " + err.Error()
				if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
					log.Printf("[Worker %d] Error adding comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
				}

				if mergeLogger != nil {
					mergeLogger.Logf("Merge failed: %v", err)
					_ = mergeLogger.End(false, err.Error())
				}

				return task.Result.Error
			}

			if mergeLogger != nil {
				mergeLogger.Logf("Merge successful")
				_ = mergeLogger.End(true, "PR merged successfully")
			}

			w.reportStageComplete("merge", EventSuccess, "PR merged successfully")

			if err := w.brMgr.CheckoutDefault(); err != nil {
				log.Printf("[Worker %d] Warning: failed to checkout default branch after merge: %v", w.id, err)
			}

			task.Status = StatusDone
			task.Result = &TaskResult{
				PRURL:   prURL,
				Summary: fmt.Sprintf("Implemented and merged #%d: %s", task.Issue.Number, task.Issue.Title),
			}
			log.Printf("[Worker %d] ✓ DONE #%d in %s → merged", w.id, task.Issue.Number, time.Since(start).Round(time.Second))
			return nil
		}

		// Decline — fix and retry
		log.Printf("[Worker %d] PR declined for #%d, fixing: %s", w.id, task.Issue.Number, decision.Reason)
		task.Status = StatusCoding
		w.reportStageComplete("coding", EventInProgress, "fixing from user decline")

		if decision.Reason != "" {
			comment := "**Declined** — sent back for fixes.\n\n" + decision.Reason
			if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
				log.Printf("[Worker %d] Error adding decline comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
			}
		}

		w.reportStageComplete("awaiting-approval", EventFailed, "user declined: "+decision.Reason)

		declineLogger, dlErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "fix-from-decline")
		if dlErr != nil {
			log.Printf("[Worker %d] Warning: failed to create decline fix logger: %v", w.id, dlErr)
		}
		if declineLogger != nil {
			_ = declineLogger.Start()
			defer declineLogger.Close()
		}
		if fixErr := w.fixFromReview(ctx, task, decision.Reason, declineLogger); fixErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("fixing from decline: %w", fixErr)}
			log.Printf("[Worker %d] ✗ FAILED fixing from decline: %v", w.id, fixErr)
			if declineLogger != nil {
				_ = declineLogger.End(false, fixErr.Error())
			}
			return task.Result.Error
		}
		if declineLogger != nil {
			_ = declineLogger.End(true, "")
		}

		if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("pushing fixes after decline: %w", pushErr)}
			return task.Result.Error
		}

		task.Status = StatusReviewing
		w.reportStageComplete("coding", EventSuccess, "fixes applied after decline")

		reReviewLogger, rrErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "code-review-after-decline")
		if rrErr != nil {
			log.Printf("[Worker %d] Warning: failed to create re-review logger: %v", w.id, rrErr)
		}
		if reReviewLogger != nil {
			_ = reReviewLogger.Start()
			defer reReviewLogger.Close()
		}
		approved, review, crErr := w.codeReview(ctx, task, prURL, reReviewLogger)
		if crErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("code review after decline: %w", crErr)}
			if reReviewLogger != nil {
				_ = reReviewLogger.End(false, crErr.Error())
			}
			return task.Result.Error
		}

		if !approved {
			task.Status = StatusCoding
			w.reportStageComplete("coding", EventInProgress, "fixing from re-review after decline")
			reFixLogger, rfErr := worker.NewStepLogger(artifactDir, task.Issue.Number, "fix-from-re-review")
			if rfErr != nil {
				log.Printf("[Worker %d] Warning: failed to create re-fix logger: %v", w.id, rfErr)
			}
			if reFixLogger != nil {
				_ = reFixLogger.Start()
				defer reFixLogger.Close()
			}
			if fixErr := w.fixFromReview(ctx, task, review, reFixLogger); fixErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("fixing from re-review: %w", fixErr)}
				if reFixLogger != nil {
					_ = reFixLogger.End(false, fixErr.Error())
				}
				return task.Result.Error
			}
			if reFixLogger != nil {
				_ = reFixLogger.End(true, "")
			}
			if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("pushing re-review fixes: %w", pushErr)}
				return task.Result.Error
			}
		}

		if reReviewLogger != nil {
			_ = reReviewLogger.End(true, fmt.Sprintf("approved=%v", approved))
		}
		w.reportStageComplete("code-review", EventSuccess, "code review passed after decline")
		// Loop back to await approval again
	}
}

func (w *Worker) technicalPlanning(ctx context.Context, task *Task, logger *worker.StepLogger) (analysis, implPlan string, err error) {
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPTechnicalPlanning), task.Issue.Number, task.Issue.Title, task.Issue.Body, task.Issue.Number)

	llmModel := w.cfg.Load().LLM.Planning.Model
	if w.router != nil {
		llmModel = w.router.SelectModel(config.CategoryPlanning, config.ComplexityMedium, nil)
	}

	response, err := w.llmStep(ctx, task, "technical-planning", prompt, llmModel, logger)
	if err != nil {
		return "", "", err
	}

	if reason := checkAlreadyDone(response); reason != "" {
		log.Printf("[Worker %d] Technical planning detected ticket already done: %s", w.id, reason)
		return "", "", fmt.Errorf("%w: %s", ErrAlreadyDone, reason)
	}

	analysis, implPlan = parseTechnicalPlanningResponse(response)

	wt := &git.Worktree{
		Name:   fmt.Sprintf("worker-%d", w.id),
		Path:   w.repoDir,
		Branch: task.Branch,
	}
	planMgr := plan.NewAttachmentManager(w.gh, wt)

	_, err = planMgr.CreateFullPlan(task.Issue.Number, task.Branch, analysis, implPlan)
	if err != nil {
		log.Printf("[Worker %d] Warning: failed to add technical planning comment: %v", w.id, err)
	} else {
		log.Printf("[Worker %d] Added technical planning comment to issue #%d", w.id, task.Issue.Number)
	}

	return analysis, implPlan, nil
}

func parseTechnicalPlanningResponse(response string) (analysis, plan string) {
	// Look for ## Analysis and ## Implementation Plan headers
	analysisIdx := strings.Index(response, "## Analysis")
	planIdx := strings.Index(response, "## Implementation Plan")

	switch {
	case analysisIdx >= 0 && planIdx > analysisIdx:
		// Extract analysis section (between ## Analysis and ## Implementation Plan)
		analysisStart := analysisIdx + len("## Analysis")
		analysis = strings.TrimSpace(response[analysisStart:planIdx])

		// Extract plan section (after ## Implementation Plan)
		planStart := planIdx + len("## Implementation Plan")
		plan = strings.TrimSpace(response[planStart:])
	case analysisIdx >= 0:
		// Only analysis header found, treat rest as plan
		analysisStart := analysisIdx + len("## Analysis")
		analysis = strings.TrimSpace(response[analysisStart:])
		plan = analysis
	case planIdx >= 0:
		// Only plan header found, treat everything before as analysis
		analysis = strings.TrimSpace(response[:planIdx])
		planStart := planIdx + len("## Implementation Plan")
		plan = strings.TrimSpace(response[planStart:])
	default:
		// No headers found, use heuristics to split
		// Try to find a natural break point (e.g., "Implementation Plan" without ##)
		lowerResponse := strings.ToLower(response)
		implIdx := strings.Index(lowerResponse, "implementation plan")
		if implIdx > 0 {
			analysis = strings.TrimSpace(response[:implIdx])
			plan = strings.TrimSpace(response[implIdx:])
		} else {
			// Can't split, use full response for both
			analysis = response
			plan = response
		}
	}

	return analysis, plan
}

func (w *Worker) implement(ctx context.Context, task *Task, planStr string, logger *worker.StepLogger) error {
	w.oc.SetDirectory(task.Worktree)

	comments, err := w.gh.ListComments(task.Issue.Number)
	if err == nil && len(comments) > 0 {
		for _, comment := range comments {
			if strings.Contains(comment.Body, "## 📋 Technical Planning") {
				planStr = comment.Body
				log.Printf("[Worker %d] Using technical planning from GitHub comment", w.id)
				break
			}
		}
	}

	if planStr == "" {
		log.Printf("[Worker %d] No technical planning comment found, using context-based plan", w.id)
	}

	testCmd := w.cfg.Load().Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPImplementation), task.Issue.Number, task.Issue.Title, planStr, task.Worktree, testCmd, task.Issue.Number, task.Issue.Number, task.Issue.Number, task.Issue.Number, task.Issue.Number)

	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		complexity := llm.DetectComplexity(planStr) //nolint:staticcheck // deprecated but still used
		llmModel = w.router.SelectModel(config.CategoryCode, complexity, nil)
	}

	_, err = w.llmStep(ctx, task, "implement", prompt, llmModel, logger)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}

func (w *Worker) ensureCommit(task *Task) {
	_, _ = git.RunInWorktree(task.Worktree, "git", "add", "-A")
	out, err := git.RunInWorktree(task.Worktree, "git", "diff", "--cached", "--quiet")
	if err != nil {
		msg := fmt.Sprintf("feat: implement #%d %s", task.Issue.Number, task.Issue.Title)
		_, _ = git.RunInWorktree(task.Worktree, "git", "commit", "-m", msg)
		log.Printf("[Worker %d] Auto-committed uncommitted changes", w.id)
	}
	_ = out
}

func (w *Worker) fixFromReview(ctx context.Context, task *Task, review string, logger *worker.StepLogger) error {
	w.oc.SetDirectory(task.Worktree)

	testCmd := w.cfg.Load().Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPFixFromReview), task.Issue.Number, task.Issue.Title, task.Worktree, testCmd, review)

	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		llmModel = w.router.SelectModel(config.CategoryCode, config.ComplexityMedium, nil)
	}

	_, err := w.llmStep(ctx, task, "fix-from-review", prompt, llmModel, logger)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}

var crSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"approved":     {"type": "boolean"},
		"issues":       {"type": "array", "items": {"type": "string"}},
		"suggestions":  {"type": "array", "items": {"type": "string"}},
		"verdict":      {"type": "string"}
	},
	"required": ["approved", "issues", "suggestions", "verdict"]
}`)

type crResult struct {
	Approved    bool     `json:"approved"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Verdict     string   `json:"verdict"`
}

func (w *Worker) codeReview(ctx context.Context, task *Task, prURL string, logger *worker.StepLogger) (approved bool, review string, err error) {
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPCodeReview), task.Issue.Number, task.Issue.Title, prURL, w.gh.Repo, task.Issue.Number)

	sessionTitle := fmt.Sprintf("code-review-%d", task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return false, "", fmt.Errorf("creating session: %w", err)
	}
	task.SetSessionID(session.ID)
	defer task.SetSessionID("")
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	var stepID int64
	if w.store != nil {
		id, sErr := w.store.InsertStep(task.Issue.Number, "code-review", prompt, session.ID)
		if sErr != nil {
			log.Printf("[Worker %d] failed to insert step: %v", w.id, sErr)
		} else {
			stepID = id
		}
	}

	task.AddChatMessage("user", prompt)

	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		hints := map[string]any{"stage": "code-review"}
		llmModel = w.router.SelectModel(config.CategoryCode, config.ComplexityHigh, hints)
	}

	model := opencode.ParseModelRef(llmModel)
	var result crResult
	if err := w.oc.SendMessageStructured(ctx, session.ID, prompt, model, crSchema, &result); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return false, "", fmt.Errorf("code review: %w", err)
	}

	reviewJSON, _ := json.Marshal(result)
	review = string(reviewJSON)

	task.AddChatMessage("assistant", review)

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, review)
	}

	if logger != nil {
		logger.Logf("Code review result: approved=%v, issues=%d, suggestions=%d", result.Approved, len(result.Issues), len(result.Suggestions))
		logger.Logf("Verdict: %s", result.Verdict)
	}

	log.Printf("[Worker %d] Code review result: %s", w.id, review)

	return result.Approved, review, nil
}

func (w *Worker) createPR(_ context.Context, task *Task, logger *worker.StepLogger) (string, error) {
	if existingBranch, err := w.gh.FindPRBranch(task.Issue.Number); err == nil && existingBranch != "" {
		log.Printf("[Worker %d] PR already exists for issue #%d (branch: %s)", w.id, task.Issue.Number, existingBranch)
	}

	var stepID int64
	if w.store != nil {
		id, err := w.store.InsertStep(task.Issue.Number, "create-pr", fmt.Sprintf("push %s + create PR", task.Branch), "")
		if err != nil {
			log.Printf("[Worker %d] failed to insert create-pr step: %v", w.id, err)
		} else {
			stepID = id
		}
	}

	if logger != nil {
		logger.Logf("Pushing branch: %s", task.Branch)
	}

	if err := w.brMgr.PushBranch(task.Branch); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		if logger != nil {
			logger.Logf("Push failed: %v", err)
		}
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	if logger != nil {
		logger.Logf("Branch pushed successfully")
	}

	body := fmt.Sprintf("Closes #%d\n\n%s", task.Issue.Number, task.Issue.Body)
	prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
	if err != nil {
		if existingURL := extractPRURL(err.Error()); existingURL != "" {
			log.Printf("[Worker %d] PR already exists: %s", w.id, existingURL)
			if w.store != nil && stepID > 0 {
				_ = w.store.FinishStep(stepID, existingURL)
			}
			if logger != nil {
				logger.Logf("Using existing PR: %s", existingURL)
			}
			return existingURL, nil
		}
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		if logger != nil {
			logger.Logf("PR creation failed: %v", err)
		}
		return "", fmt.Errorf("creating PR: %w", err)
	}

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, prURL)
	}

	if logger != nil {
		logger.Logf("PR created: %s", prURL)
	}

	return prURL, nil
}

func (w *Worker) checkPipeline(ctx context.Context, task *Task, logger *worker.StepLogger) error {
	cfg := w.cfg.Load()
	interval := time.Duration(cfg.Pipeline.CheckInterval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	timeout := time.Duration(cfg.Pipeline.CheckTimeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	if logger != nil {
		logger.Logf("Starting pipeline check (timeout: %s, interval: %s)", timeout, interval)
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if logger != nil {
				logger.Logf("Pipeline check canceled: %v", ctx.Err())
			}
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				if logger != nil {
					logger.Logf("Pipeline check timed out after %s", timeout)
				}
				return w.handlePipelineFailure(task, "CI checks timed out after "+timeout.String())
			}

			result, err := w.gh.GetPRChecks(task.Branch)
			if err != nil {
				log.Printf("[Worker %d] Error checking PR checks: %v", w.id, err)
				if logger != nil {
					logger.Logf("Error checking PR checks: %v", err)
				}
				continue
			}

			switch result.Status {
			case "pass":
				log.Printf("[Worker %d] All CI checks passed for #%d", w.id, task.Issue.Number)
				if logger != nil {
					logger.Logf("All CI checks passed")
				}
				return nil
			case "fail":
				if logger != nil {
					logger.Logf("CI checks failed: %s", worker.Truncate(result.Logs, 500))
				}
				return w.handlePipelineFailure(task, result.Logs)
			case "pending":
				log.Printf("[Worker %d] CI checks still pending for #%d...", w.id, task.Issue.Number)
				if logger != nil {
					logger.Logf("CI checks still pending...")
				}
			}
		}
	}
}

func (w *Worker) handlePipelineFailure(task *Task, logs string) error {
	artifactDir := filepath.Join(w.repoDir, ".oda", "artifacts", strconv.Itoa(task.Issue.Number))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		log.Printf("[Worker %d] Warning: failed to create artifact dir: %v", w.id, err)
	}

	logPath := filepath.Join(artifactDir, "pipeline-fail.log")
	if err := os.WriteFile(logPath, []byte(logs), 0o644); err != nil {
		log.Printf("[Worker %d] Warning: failed to save pipeline failure log: %v", w.id, err)
	}

	comment := fmt.Sprintf("## CI Pipeline Failed\n\nPipeline checks failed. Logs saved to `%s`.\n\n```\n%s\n```", logPath, logs)
	if err := w.gh.AddComment(task.Issue.Number, comment); err != nil {
		log.Printf("[Worker %d] Warning: failed to add pipeline failure comment: %v", w.id, err)
	}

	return errors.New("CI pipeline checks failed")
}

func (w *Worker) llmStep(_ context.Context, task *Task, stepName, prompt, llm string, logger *worker.StepLogger) (string, error) {
	sessionTitle := fmt.Sprintf("%s-%d", stepName, task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	task.SetSessionID(session.ID)
	defer task.SetSessionID("")
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	var stepID int64
	if w.store != nil {
		id, sErr := w.store.InsertStep(task.Issue.Number, stepName, prompt, session.ID)
		if sErr != nil {
			log.Printf("[Worker %d] failed to insert step: %v", w.id, sErr)
		} else {
			stepID = id
		}
	}

	task.AddChatMessage("user", prompt)

	model := opencode.ParseModelRef(llm)
	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("sending message: %w", err)
	}

	if logger != nil {
		logger.LogLLMResponse(msg)
	}

	response := extractText(msg)

	task.AddChatMessage("assistant", response)

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, response)
	}

	return response, nil
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

var prURLRe = regexp.MustCompile(`https://github\.com/[^\s]+/pull/\d+`)

func extractPRURL(errMsg string) string {
	match := prURLRe.FindString(errMsg)
	return match
}

func checkAlreadyDone(response string) string {
	for line := range strings.SplitSeq(response, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "ALREADY_DONE:"); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
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

func (w *Worker) createArtifactDir(issueNumber int) (string, error) {
	artifactDir := filepath.Join(w.repoDir, ".oda", "artifacts", strconv.Itoa(issueNumber))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", fmt.Errorf("creating artifact directory %s: %w", artifactDir, err)
	}
	return artifactDir, nil
}

// UpdateConfig updates the worker's configuration atomically.
// This method is called by the ConfigPropagator when config changes.
func (w *Worker) UpdateConfig(cfg *config.Config) {
	w.cfg.Store(cfg)
	log.Printf("[Worker %d] Configuration updated (YoloMode=%v)", w.id, cfg.YoloMode)
}

// Compile-time interface check: ensure Worker implements ConfigAwareWorker.
var _ config.ConfigAwareWorker = (*Worker)(nil)
