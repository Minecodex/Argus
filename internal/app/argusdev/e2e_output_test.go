package argusdev

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDiagnosticStreamRedactsAcrossEveryWriteBoundary(t *testing.T) {
	input := "installed\r\nhttps://platform.test/#argus_setup_token=private-fixture\r\nready\nAuthorization: Bearer private-fixture\nfinal"
	for size := 1; size <= len(input); size++ {
		var output bytes.Buffer
		writer := &diagnosticStream{destination: &output}
		for start := 0; start < len(input); start += size {
			chunk := input[start:min(start+size, len(input))]
			if n, err := writer.Write([]byte(chunk)); err != nil || n != len(chunk) {
				t.Fatalf("chunk %d: written %d, error %v", size, n, err)
			}
		}
		if err := writer.Flush(); err != nil {
			t.Fatal(err)
		}
		if output.String() != "installed\r\nready\nfinal" {
			t.Fatalf("chunk %d exposed or lost output: %q", size, output.String())
		}
	}
}

func TestDiagnosticStreamBoundsIncompleteLines(t *testing.T) {
	var output bytes.Buffer
	writer := &diagnosticStream{destination: &output}
	_, err := writer.Write([]byte(strings.Repeat("x", diagnosticLineLimit+1) + " token=private\nready\nsecret=unfinished"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "ready\n" {
		t.Fatalf("unexpected bounded output: %q", output.String())
	}
}

type shortDiagnosticWriter struct{}

func (shortDiagnosticWriter) Write([]byte) (int, error) { return 0, nil }

func TestDiagnosticStreamPropagatesOutputFailure(t *testing.T) {
	writer := &diagnosticStream{destination: shortDiagnosticWriter{}}
	if _, err := writer.Write([]byte("ready\n")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("output error was ignored: %v", err)
	}
}
