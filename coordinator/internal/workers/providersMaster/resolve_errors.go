package providersmaster

import (
	"errors"
	"fmt"

	"mytonprovider-coordinator/internal/pipelineevents"
)

type resolveStageError struct {
	Stage string
	Err   error
}

func (e *resolveStageError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("resolve failed at stage %s", e.Stage)
	}
	return e.Err.Error()
}

func (e *resolveStageError) Unwrap() error {
	return e.Err
}

func stageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &resolveStageError{Stage: stage, Err: err}
}

func resolveStageFromErr(err error) string {
	if err == nil {
		return pipelineevents.StageUnknown
	}
	var se *resolveStageError
	if errors.As(err, &se) && se.Stage != "" {
		return se.Stage
	}
	return pipelineevents.StageUnknown
}
