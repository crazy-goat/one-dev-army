package pipeline

import "fmt"

type StageResult struct {
	Stage   Stage
	Success bool
	Output  string
	Error   error
}

type StageExecutor interface {
	Execute(taskID int, stage Stage, context string) (*StageResult, error)
}

type Pipeline struct {
	maxRetries    int
	executor      StageExecutor
	onStageChange func(int, Stage)
}

func New(maxRetries int, executor StageExecutor, onStageChange func(int, Stage)) *Pipeline {
	return &Pipeline{
		maxRetries:    maxRetries,
		executor:      executor,
		onStageChange: onStageChange,
	}
}

func (p *Pipeline) Run(taskID int, startStage Stage, context string) (*StageResult, error) {
	stage := startStage
	retries := 0
	var failedStage Stage

	for {
		if p.onStageChange != nil {
			p.onStageChange(taskID, stage)
		}

		result, err := p.executor.Execute(taskID, stage, context)
		if err != nil {
			return nil, fmt.Errorf("executing stage %s: %w", stage, err)
		}

		if result.Success {
			// Pass output from this stage as context to the next stage,
			// so each stage builds on the previous one's work
			// (e.g., analysis plan feeds into coding stage).
			if result.Output != "" {
				context = result.Output
			}

			next := stage.Next()
			if next == stage || stage == StageDone {
				result.Stage = StageDone
				return result, nil
			}
			if failedStage != "" && stage == failedStage {
				retries = 0
				failedStage = ""
			}
			stage = next
			if stage == StageDone {
				if p.onStageChange != nil {
					p.onStageChange(taskID, stage)
				}
				result.Stage = StageDone
				return result, nil
			}
			continue
		}

		retries++
		if retries >= p.maxRetries {
			stage = StageBlocked
			if p.onStageChange != nil {
				p.onStageChange(taskID, stage)
			}
			result.Stage = StageBlocked
			return result, nil
		}

		if failedStage == "" {
			failedStage = stage
		}
		stage = stage.RetryTarget()
	}
}
