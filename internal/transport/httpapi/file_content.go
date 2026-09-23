package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type fileContentResponse struct {
	reader         io.ReadCloser
	size           int64
	name, hash     string
	requestedRange *string
}

func (response fileContentResponse) VisitDownloadWorkspaceFileResponse(w http.ResponseWriter) error {
	return response.write(w)
}
func (response fileContentResponse) VisitDownloadFileDeliveryResponse(w http.ResponseWriter) error {
	return response.write(w)
}
func (response fileContentResponse) write(w http.ResponseWriter) error {
	defer response.reader.Close()
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": response.name}))
	if response.hash != "" {
		w.Header().Set("ETag", `"sha256:`+response.hash+`"`)
	}
	start, length := int64(0), response.size
	status := http.StatusOK
	if response.requestedRange != nil {
		var err error
		start, length, err = singleFileRange(*response.requestedRange, response.size)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", response.size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return nil
		}
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, response.size))
	}
	if _, err := io.CopyN(io.Discard, response.reader, start); err != nil {
		return err
	}
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.WriteHeader(status)
	_, err := io.CopyN(w, response.reader, length)
	return err
}
func singleFileRange(value string, size int64) (int64, int64, error) {
	invalid := errors.New("unsupported or unsatisfiable byte range")
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") || size <= 0 {
		return 0, 0, invalid
	}
	first, last, ok := strings.Cut(strings.TrimPrefix(value, "bytes="), "-")
	if !ok {
		return 0, 0, invalid
	}
	number := func(value string) (int64, error) {
		if value == "" || strings.Trim(value, "0123456789") != "" {
			return 0, invalid
		}
		return strconv.ParseInt(value, 10, 64)
	}
	if first == "" {
		suffix, err := number(last)
		if err != nil || suffix <= 0 {
			return 0, 0, invalid
		}
		suffix = min(suffix, size)
		return size - suffix, suffix, nil
	}
	start, err := number(first)
	if err != nil || start >= size {
		return 0, 0, invalid
	}
	end := size - 1
	if last != "" {
		end, err = number(last)
		if err != nil || end < start {
			return 0, 0, invalid
		}
		end = min(end, size-1)
	}
	return start, end - start + 1, nil
}
