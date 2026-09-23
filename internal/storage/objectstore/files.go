package objectstore

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
)

// File transfers are streaming and have their own budget. Recording chunks
// retain their existing bounded API; large files never pass through []byte Get.
func (client *Client) PutFile(ctx context.Context, key string, reader io.Reader, size int64, mediaType string) error {
	if client == nil || client.client == nil || key == "" || size < 0 {
		return errors.New("file object store unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_, err := client.client.PutObject(ctx, client.bucket, key, reader, size, minio.PutObjectOptions{ContentType: mediaType, DisableMultipart: size < 16<<20})
	return err
}

func (client *Client) ReadFile(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if client == nil || client.client == nil || key == "" || offset < 0 || length < 0 {
		return nil, errors.New("file object store unavailable")
	}
	opts := minio.GetObjectOptions{}
	if length > 0 {
		if err := opts.SetRange(offset, offset+length-1); err != nil {
			return nil, err
		}
	} else if offset > 0 {
		if err := opts.SetRange(offset, 0); err != nil {
			return nil, err
		}
	}
	object, err := client.client.GetObject(ctx, client.bucket, key, opts)
	if err != nil {
		return nil, err
	}
	if _, err := object.Stat(); err != nil {
		object.Close()
		return nil, err
	}
	return object, nil
}

func (client *Client) DeleteFile(ctx context.Context, key string) error {
	if client == nil || client.client == nil || key == "" {
		return errors.New("file object store unavailable")
	}
	return client.client.RemoveObject(ctx, client.bucket, key, minio.RemoveObjectOptions{})
}

func (client *Client) DeletePrefix(ctx context.Context, prefix string) error {
	if client == nil || client.client == nil || len(prefix) < 16 {
		return errors.New("invalid file object prefix")
	}
	for object := range client.client.ListObjects(ctx, client.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if err := client.DeleteFile(ctx, object.Key); err != nil {
			return err
		}
	}
	return nil
}
