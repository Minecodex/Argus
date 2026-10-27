package kubernetesstream

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

func (s *Session) runExec(cols, rows int) error {
	read := s.pipeRead
	defer read.Close()
	s.resize <- remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	request := s.api.CoreV1().RESTClient().Post().Namespace(s.target.Pin.Namespace).Resource("pods").Name(s.target.Pin.Pod).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: s.target.Pin.Container, Command: ShellCommand(s.target.Shell), Stdin: true, Stdout: true, Stderr: false, TTY: true}, scheme.ParameterCodec)
	transport, upgrader, err := spdy.RoundTripperFor(s.config)
	if err != nil {
		return err
	}
	executor, err := remotecommand.NewSPDYExecutorRejectRedirects(transport, upgrader, http.MethodPost, request.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(s.ctx, remotecommand.StreamOptions{Stdin: read, Stdout: outputWriter{s, "stdout"}, Tty: true, TerminalSizeQueue: terminalQueue{s}})
}
