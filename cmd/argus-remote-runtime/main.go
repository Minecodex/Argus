package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/remoteexecution/runner"
	"github.com/kakj-go/Argus/internal/remoteexecution/runtimewire"
	"net"
	"os"
	"os/signal"
	"time"
)

func main() {
	resource := flag.String("kubernetes-resource-id", "", "Approved resource ID")
	interpreter := flag.String("interpreter", "", "kubectl or pod_bash")
	podUID := flag.String("pod-uid", "", "Pinned Pod UID")
	base := flag.String("directory", "", "Session temporary directory")
	probe := flag.String("probe-tcp", "", "Trusted connectivity probe")
	sentinel := flag.Bool("isolation-sentinel", false, "Trusted isolation preflight listener")
	flag.Parse()
	if *probe != "" && flag.NArg() == 0 {
		conn, err := net.DialTimeout("tcp", *probe, 2*time.Second)
		if err != nil {
			os.Exit(1)
		}
		conn.Close()
		return
	}
	if *sentinel && flag.NArg() == 0 {
		listener, err := net.Listen("tcp4", ":39999")
		if err != nil {
			os.Exit(2)
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				os.Exit(2)
			}
			conn.Close()
		}
	}
	if _, err := uuid.Parse(*resource); err != nil || (*interpreter != "kubectl" && *interpreter != "pod_bash") || (*interpreter == "pod_bash" && *podUID == "") || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "REMOTE_BINDING_INVALID")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	manager := runner.New(*base)
	manager.KubernetesResourceID = *resource
	manager.KubernetesInterpreter = *interpreter
	manager.KubernetesPodUID = *podUID
	if err := runtimewire.Serve(ctx, manager, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "REMOTE_RUNTIME_FAILED")
		os.Exit(1)
	}
}
