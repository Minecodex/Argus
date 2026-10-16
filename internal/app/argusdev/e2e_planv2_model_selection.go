package argusdev

import "context"

func (a *App) verifyPlanV2ModelSelection(ctx context.Context, env *E2EEnvironment) (p5BenchmarkSample, error) {
	samples, err := a.planV2AutomaticTasks(ctx, env, false)
	if len(samples) == 0 {
		return p5BenchmarkSample{}, err
	}
	return samples[0], err
}
