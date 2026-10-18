package runner

import (
	"bufio"
	"errors"
)

// Control messages have independent framing and a hard allocation ceiling.
func readBoundedFrame(reader *bufio.Reader, delimiter byte, limit int) ([]byte, error) {
	var out []byte
	for {
		part, err := reader.ReadSlice(delimiter)
		if len(out)+len(part) > limit {
			return nil, errors.New("REMOTE_COMPLETION_INVALID")
		}
		out = append(out, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return out, err
	}
}
