//go:build windows

package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

func platformSelfTest(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	terminal, err := openLocalTerminal(ctx, 100, 30)
	if err != nil {
		return fmt.Errorf("open ConPTY: %w", err)
	}
	defer terminal.Close()
	if err = terminal.Resize(120, 40); err != nil {
		return fmt.Errorf("resize ConPTY: %w", err)
	}
	const marker = "ARGUS_CONPTY_SELF_TEST_OK"
	chunks := make(chan []byte, 4)
	readErrors := make(chan error, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, readErr := terminal.Read(buffer)
			if count > 0 {
				value := append([]byte(nil), buffer[:count]...)
				select {
				case chunks <- value:
				case <-ctx.Done():
					return
				}
			}
			if readErr != nil {
				readErrors <- readErr
				return
			}
		}
	}()
	encodedMarker := base64.StdEncoding.EncodeToString([]byte(marker))
	command := "[Console]::OutputEncoding=[Text.UTF8Encoding]::new(); Write-Output ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encodedMarker + "'))); exit\r\n"
	if _, err = terminal.Write([]byte(command)); err != nil {
		return fmt.Errorf("write ConPTY: %w", err)
	}
	var output []byte
	for {
		select {
		case value := <-chunks:
			output = append(output, value...)
			if bytes.Contains(output, []byte(marker)) {
				return nil
			}
		case readErr := <-readErrors:
			if bytes.Contains(output, []byte(marker)) {
				return nil
			}
			return fmt.Errorf("read ConPTY: %w", readErr)
		case <-ctx.Done():
			return errors.New("ConPTY self-test timed out")
		}
	}
}
