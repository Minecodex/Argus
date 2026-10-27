package builtincollection

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

type ComponentTLS struct {
	CAPEM      string `json:"ca_pem,omitempty"`
	ServerName string `json:"server_name,omitempty"`
}

type Config struct {
	Interval     string        `json:"interval"`
	ComponentTLS *ComponentTLS `json:"component_tls,omitempty"`
}

func HasComponentTLS(key string) bool {
	definition, ok := Lookup(key)
	return ok && definition.ComponentTLS
}

// ParseFor is shared by the domain and compiler. Fixed sources never accept an
// arbitrary URL, bearer token, client key, or disabled certificate verification.
func ParseFor(key string, raw []byte) (Config, error) {
	var value Config
	if _, ok := Lookup(key); !ok {
		return value, errors.New("unknown builtin collection")
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 288<<10 || decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return value, errors.New("invalid builtin configuration")
	}
	if value.Interval == "" {
		value.Interval = "30s"
	}
	interval, err := time.ParseDuration(value.Interval)
	if err != nil || interval < 5*time.Second || interval > time.Hour {
		return value, errors.New("invalid builtin collection interval")
	}
	value.Interval = interval.String()
	if value.ComponentTLS != nil {
		if !HasComponentTLS(key) {
			return value, errors.New("component TLS is not applicable to this builtin")
		}
		tls := value.ComponentTLS
		tls.ServerName = strings.TrimSpace(tls.ServerName)
		if tls.ServerName != "" && !validTLSName(tls.ServerName) {
			return value, errors.New("invalid component certificate name")
		}
		if len(tls.CAPEM) > 256<<10 {
			return value, errors.New("component trust bundle exceeds budget")
		}
		if strings.TrimSpace(tls.CAPEM) != "" {
			rest := []byte(tls.CAPEM)
			var normalized bytes.Buffer
			count := 0
			for len(bytes.TrimSpace(rest)) > 0 {
				if !bytes.HasPrefix(bytes.TrimSpace(rest), []byte("-----BEGIN CERTIFICATE-----")) {
					return value, errors.New("only public certificate PEM blocks are accepted")
				}
				block, next := pem.Decode(rest)
				if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) > 0 {
					return value, errors.New("only public certificate PEM blocks are accepted")
				}
				if _, err = x509.ParseCertificate(block.Bytes); err != nil {
					return value, errors.New("invalid public component certificate")
				}
				count++
				if count > 32 {
					return value, errors.New("component trust bundle exceeds budget")
				}
				if err = pem.Encode(&normalized, block); err != nil {
					return value, errors.New("invalid public component certificate")
				}
				rest = next
			}
			tls.CAPEM = normalized.String()
		} else {
			tls.CAPEM = ""
		}
		if tls.CAPEM == "" && tls.ServerName == "" {
			value.ComponentTLS = nil
		}
	}
	return value, nil
}

func validTLSName(name string) bool {
	if len(name) > 253 {
		return false
	}
	if net.ParseIP(name) != nil {
		return true
	}
	for _, part := range strings.Split(name, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, r := range part {
			if r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}
