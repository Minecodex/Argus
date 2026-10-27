package kubernetespodlogs

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Kubernetes serializes SinceTime at second precision. Preserve the exact
// prefix at the boundary second instead of deduplicating equal log text, which
// would discard legitimate repeated messages. Rotation/changed prefixes stop
// the session explicitly; they must never be treated as an empty success.
type Watermark struct {
	Second time.Time `json:"second"`
	Count  int       `json:"count"`
	Hash   string    `json:"hash"`
}

func emptyWatermark(second time.Time, key []byte) Watermark {
	hash := hmac.New(sha256.New, key)
	return Watermark{Second: second.UTC().Truncate(time.Second), Hash: hex.EncodeToString(hash.Sum(nil))}
}

func Continue(raw []byte, q Query, at time.Time) (Batch, error) {
	result := Batch{ObservedAt: at, Next: emptyWatermark(q.Since, q.ContinuityKey), Truncated: len(raw) > MaxBytes}
	if q.Resume != nil {
		result.Next = *q.Resume
	}
	if len(raw) > MaxBytes {
		raw = raw[:MaxBytes]
	}
	lines := bytes.SplitAfter(raw, []byte{'\n'})
	type entry struct {
		text []byte
		at   time.Time
	}
	entries := make([]entry, 0, len(lines))
	var previous time.Time
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[len(line)-1] != '\n' && !q.Pin.Previous {
			result.PendingLine = true
			break
		}
		separator := bytes.IndexByte(line, ' ')
		if separator < 1 {
			return Batch{}, ErrGap
		}
		stamp, err := time.Parse(time.RFC3339Nano, string(line[:separator]))
		if err != nil || stamp.Before(q.Since) || !previous.IsZero() && stamp.Before(previous) {
			return Batch{}, ErrGap
		}
		previous = stamp
		entries = append(entries, entry{line, stamp})
	}
	offset := 0
	if q.Resume != nil {
		prefix := hmac.New(sha256.New, q.ContinuityKey)
		count := 0
		for offset < len(entries) && entries[offset].at.Truncate(time.Second).Equal(q.Resume.Second) && count < q.Resume.Count {
			_, _ = prefix.Write(entries[offset].text)
			count++
			offset++
		}
		if count != q.Resume.Count || hex.EncodeToString(prefix.Sum(nil)) != q.Resume.Hash {
			return Batch{}, ErrGap
		}
	}
	if offset < len(entries) {
		second := entries[len(entries)-1].at.UTC().Truncate(time.Second)
		hash := hmac.New(sha256.New, q.ContinuityKey)
		count := 0
		for _, entry := range entries {
			if entry.at.Truncate(time.Second).Equal(second) {
				_, _ = hash.Write(entry.text)
				count++
			}
		}
		result.Next = Watermark{Second: second, Count: count, Hash: hex.EncodeToString(hash.Sum(nil))}
	}
	output := entries[offset:]
	if q.Resume == nil && len(output) > q.TailLines {
		output = output[len(output)-q.TailLines:]
	}
	var body strings.Builder
	for _, line := range output {
		body.Write(line.text)
	}
	result.Content = strings.ToValidUTF8(body.String(), "\ufffd")
	return result, nil
}
