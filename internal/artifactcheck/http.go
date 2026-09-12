package artifactcheck

import (
	"context"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/artifacthttp"
	"net/http"
	"time"
)

var ErrUnavailable = errors.New("artifact object is unavailable")

type Checker interface {
	Check(context.Context, ...string) error
}

type HTTPChecker struct{ client *http.Client }

func NewHTTPChecker(caPath string) (*HTTPChecker, error) {
	client, err := artifacthttp.FromEnvironment(caPath)
	if err != nil {
		return nil, err
	}
	client.Timeout = 10 * time.Second
	return &HTTPChecker{client: client}, nil
}
func (checker *HTTPChecker) Check(ctx context.Context, urls ...string) error {
	if checker == nil || checker.client == nil {
		return ErrUnavailable
	}
	for _, rawURL := range urls {
		request, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
		if err != nil {
			return fmt.Errorf("%w: invalid release URL", ErrUnavailable)
		}
		response, err := checker.client.Do(request)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
		}
	}
	return nil
}
