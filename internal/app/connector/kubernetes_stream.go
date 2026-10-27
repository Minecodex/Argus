package connector

import (
	"context"
	"encoding/json"
	"slices"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
)

func interactiveCapabilities(role string, existing []string) []string {
	values := append([]string(nil), existing...)
	if (role == "kubernetes" || role == "bastion") && !slices.Contains(values, kubernetesstream.SchemaVersion) {
		values = append(values, kubernetesstream.SchemaVersion)
	}
	if (role == "kubernetes" || role == "bastion") && !slices.Contains(values, kuberneteshelm.SchemaVersion) {
		values = append(values, kuberneteshelm.SchemaVersion)
	}
	return values
}

type kubernetesStreamEnd struct{ status, reason string }

func (e *kubernetesStreamEnd) Error() string { return e.reason }

func (s *localRemoteSession) runKubernetesStream(ctx context.Context, credential []byte, outgoing chan<- outboundFrame) error {
	var target kubernetesstream.Target
	if len(s.open.KubernetesTargetJson) > 16<<10 || json.Unmarshal(s.open.KubernetesTargetJson, &target) != nil || target.Validate() != nil {
		return kubernetesstream.ErrProtocol
	}
	if s.open.Protocol != "kubernetes_"+target.Mode {
		return kubernetesstream.ErrProtocol
	}
	config, err := connectorKubeconfig(credential, target.APIServer)
	if err != nil {
		return err
	}
	backend, err := kubernetesstream.Open(ctx, config, target, int(s.open.TerminalCols), int(s.open.TerminalRows))
	if err != nil {
		return err
	}
	defer backend.Close(context.Background(), "connector_closed")
	select {
	case outgoing <- outboundFrame{value: s.stateFrame(connectorv1.RemoteAccessStateValue_REMOTE_ACCESS_STATE_VALUE_ACTIVE, "")}:
	case <-ctx.Done():
		return ctx.Err()
	}
	frames := make(chan interactive.Frame, 1)
	failures := make(chan error, 1)
	go func() {
		for {
			frame, e := backend.Receive(ctx)
			if e != nil {
				select {
				case failures <- e:
				case <-ctx.Done():
				}
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-failures:
			return err
		case frame := <-frames:
			if frame.Type == "state" {
				return &kubernetesStreamEnd{status: frame.Status, reason: frame.Reason}
			}
			if len(frame.Data) > 0 {
				select {
				case outgoing <- outboundFrame{value: s.outputFrame(connectorv1.RemoteAccessOutputStream_REMOTE_ACCESS_OUTPUT_STREAM_STDOUT, frame.Data)}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		case request := <-s.inbound:
			var input interactive.Input
			switch {
			case request.GetRemoteAccessInput() != nil:
				input = interactive.Input{Type: "input", Data: string(request.GetRemoteAccessInput().Data)}
			case request.GetRemoteAccessResize() != nil:
				resize := request.GetRemoteAccessResize()
				input = interactive.Input{Type: "resize", Cols: int(resize.Cols), Rows: int(resize.Rows)}
			case request.GetRemoteAccessClose() != nil:
				return nil
			default:
				return kubernetesstream.ErrProtocol
			}
			if err = backend.Send(ctx, input); err != nil {
				return err
			}
		}
	}
}
