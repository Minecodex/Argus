// Publish one versioned signed artifact using the existing object-store SDK.
package main

import (
	"context"
	"encoding/json"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"os"
)

func main() {
	var in struct{ Endpoint, Access, Secret, Path, Key string }
	file, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic("private publication input unavailable")
	}
	defer clear(file)
	if json.Unmarshal(file, &in) != nil {
		panic("invalid publication input")
	}
	client, err := minio.New(in.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(in.Access, in.Secret, ""), Region: "us-east-1"})
	if err != nil {
		panic("artifact client unavailable")
	}
	_, err = client.StatObject(context.Background(), "argus-collector-artifacts", in.Key, minio.StatObjectOptions{})
	if err == nil {
		panic("artifact object already exists; inspect its digest before resuming")
	}
	_, err = client.FPutObject(context.Background(), "argus-collector-artifacts", in.Key, in.Path, minio.PutObjectOptions{ContentType: "application/gzip"})
	if err != nil {
		panic("artifact upload failed")
	}
	println("Versioned signed Collector artifact uploaded")
}
