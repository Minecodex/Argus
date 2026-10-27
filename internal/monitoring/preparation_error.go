package monitoring

import "errors"

type preparationError struct {
	stage string
	cause error
}

func (e preparationError) Error() string          { return "monitoring preparation failed at " + e.stage }
func (e preparationError) Unwrap() error          { return e.cause }
func (e preparationError) OperationStage() string { return e.stage }
func prepareFailure(stage string, err error) error {
	if err == nil {
		return nil
	}
	if PreparationStage(err) != "" {
		return err
	}
	return preparationError{stage: stage, cause: err}
}

// PreparationStage exposes only a controlled operation label, never driver
// messages, endpoints, authentication material, SQL or response bodies.
func PreparationStage(err error) string {
	var staged interface{ OperationStage() string }
	if errors.As(err, &staged) {
		return staged.OperationStage()
	}
	return ""
}
