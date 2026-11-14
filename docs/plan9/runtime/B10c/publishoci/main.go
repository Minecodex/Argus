// publishoci publishes real packaged Helm artifacts using the native ORAS SDK.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	repository := flag.String("repository", "", "Owned registry and repository path")
	directory := flag.String("charts", "", "Packaged Chart directory")
	account := flag.String("account", "", "Private Basic authentication JSON file")
	ca := flag.String("ca", "", "Trusted public CA PEM file")
	flag.Parse()
	if *repository == "" || *directory == "" || *account == "" || *ca == "" {
		return fmt.Errorf("all publisher flags are required")
	}
	var credentials struct{ Username, Password string }
	data, err := os.ReadFile(*account)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &credentials); err != nil {
		return err
	}
	pem, err := os.ReadFile(*ca)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("invalid public source CA")
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
	repo, err := remote.NewRepository(*repository)
	if err != nil {
		return err
	}
	repo.PlainHTTP = false
	host := strings.SplitN(*repository, "/", 2)[0]
	repo.Client = &auth.Client{Client: client, Cache: auth.NewCache(), Credential: auth.StaticCredential(host,
		auth.Credential{Username: credentials.Username, Password: credentials.Password})}
	for _, version := range []string{"0.1.0", "0.2.0"} {
		if err = publish(repo, *directory, version); err != nil {
			return err
		}
	}
	return nil
}

func publish(repo *remote.Repository, directory, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store := memory.New()
	push := func(data []byte, media string) (ocispec.Descriptor, error) {
		d := ocispec.Descriptor{MediaType: media, Size: int64(len(data)), Digest: digest.FromBytes(data)}
		return d, store.Push(ctx, d, bytes.NewReader(data))
	}
	config, err := json.Marshal(map[string]string{"name": "b10c-web", "version": version, "apiVersion": "v2"})
	if err != nil {
		return err
	}
	cfg, err := push(config, "application/vnd.cncf.helm.config.v1+json")
	if err != nil {
		return err
	}
	archive, err := os.ReadFile(filepath.Join(directory, "b10c-web-"+version+".tgz"))
	if err != nil {
		return err
	}
	layer, err := push(archive, "application/vnd.cncf.helm.chart.content.v1.tar+gzip")
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest, Config: cfg, Layers: []ocispec.Descriptor{layer}})
	if err != nil {
		return err
	}
	d, err := push(manifest, ocispec.MediaTypeImageManifest)
	if err != nil {
		return err
	}
	if err = store.Tag(ctx, d, version); err != nil {
		return err
	}
	if _, err = oras.Copy(ctx, store, version, repo, version, oras.DefaultCopyOptions); err != nil {
		return err
	}
	fmt.Println(version, d.Digest.String())
	return nil
}
