package argusctl

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kakj-go/Argus/internal/kubernetesstreamclient"
)

func (a *App) runKubernetesForward(ctx context.Context, args []string) error {
	f := flag.NewFlagSet("kubernetes port-forward", flag.ContinueOnError)
	f.SetOutput(a.stderr)
	endpoint := f.String("url", "", "Kubernetes session WebSocket URL")
	stdin := f.Bool("ticket-stdin", false, "read a one-use ticket from standard input")
	listen := f.String("listen", "127.0.0.1:18080", "local loopback listener")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || !*stdin || *endpoint == "" {
		return errors.New("usage: argusctl kubernetes port-forward --url URL --ticket-stdin --listen 127.0.0.1:PORT")
	}
	if err := kubernetesstreamclient.ValidateURL(*endpoint); err != nil {
		return err
	}
	listener, err := kubernetesstreamclient.Listen(*listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	_, _ = fmt.Fprintln(a.stderr, "Paste the one-use session ticket and press Enter:")
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 128)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return errors.New("cannot read session ticket")
	}
	ticket := strings.TrimSpace(line)
	line = ""
	if len(ticket) != 43 {
		return errors.New("invalid one-use ticket")
	}
	err = kubernetesstreamclient.Forward(ctx, listener, kubernetesstreamclient.Options{URL: *endpoint, Ticket: ticket, Ready: func() {
		_, _ = fmt.Fprintf(a.stdout, "Forwarding %s through the pinned Kubernetes session. Press Ctrl+C to stop.\n", listener.Addr())
	}})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
