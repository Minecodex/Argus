package main

import (
	"fmt"
	"github.com/kakj-go/Argus/internal/remoteexecution/kubectl"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"os"
)

func main() {
	if err := kubectl.Run(os.Args[1:], genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
