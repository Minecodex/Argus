package alerting

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebhookProtocolsRequireBusinessAcknowledgement(t *testing.T) {
	stamp := time.Unix(1800000000, 0).UTC()
	for _, provider := range []string{"dingtalk", "wecom", "feishu"} {
		t.Run(provider, func(t *testing.T) {
			var mu sync.Mutex
			received := []map[string]any{}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid protocol JSON")
				}
				mu.Lock()
				received = append(received, body)
				mu.Unlock()
				if provider == "dingtalk" {
					s := strconv.FormatInt(stamp.UnixMilli(), 10)
					mac := hmac.New(sha256.New, []byte("fixture-secret"))
					_, _ = mac.Write([]byte(s + "\nfixture-secret"))
					if r.URL.Query().Get("timestamp") != s || r.URL.Query().Get("sign") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
						t.Error("DingTalk signed request not accepted")
					}
				}
				if provider == "feishu" {
					s := strconv.FormatInt(stamp.Unix(), 10)
					mac := hmac.New(sha256.New, []byte(s+"\nfixture-secret"))
					if body["timestamp"] != s || body["sign"] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
						t.Error("Feishu signed request not accepted")
					}
				}
				switch r.URL.Path {
				case "/ok":
					if provider == "feishu" {
						_, _ = io.WriteString(w, `{"code":0}`)
					} else {
						_, _ = io.WriteString(w, `{"errcode":0}`)
					}
				case "/rejected":
					if provider == "feishu" {
						_, _ = io.WriteString(w, `{"code":19024}`)
					} else {
						_, _ = io.WriteString(w, `{"errcode":310000}`)
					}
				case "/limited":
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(429)
				default:
					_, _ = io.WriteString(w, `{}`)
				}
			}))
			defer server.Close()
			transport := Transport{Client: server.Client(), Now: func() time.Time { return stamp }, ValidateEndpoint: func(_ context.Context, url string) error {
				if !strings.HasPrefix(url, server.URL+"/") {
					return ErrDenied
				}
				return nil
			}}
			for _, tc := range []struct {
				path                     string
				accepted, retry, unknown bool
			}{{"/ok", true, false, false}, {"/rejected", false, false, false}, {"/limited", false, true, false}, {"/empty", false, false, true}} {
				result := transport.Send(t.Context(), ChannelSpec{Provider: provider, MentionIDs: []string{"person-1"}, MentionAll: true}, NotificationCredential{WebhookURL: server.URL + tc.path, SigningSecret: "fixture-secret"}, Message{Text: "CPU 95%"})
				if result.Accepted != tc.accepted || result.Retryable != tc.retry || result.Unknown != tc.unknown {
					t.Fatal(tc, result)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if len(received) != 4 {
				t.Fatal("receiver did not record requests")
			}
			t.Logf("receiver recorded %d signed %s protocol requests", len(received), provider)
		})
	}
}

func TestSMTPReceiverRecordsBodyAndRequiresDataAcknowledgement(t *testing.T) {
	certificateSource := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certificateSource.Close()
	roots := x509.NewCertPool()
	roots.AddCert(certificateSource.Certificate())
	for _, ack := range []bool{true, false} {
		t.Run(fmt.Sprintf("ack=%t", ack), func(t *testing.T) {
			listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certificateSource.TLS.Certificates, MinVersion: tls.VersionTLS12})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			received := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				_, _ = io.WriteString(conn, "220 fixture ESMTP\r\n")
				body := strings.Builder{}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						done <- nil
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						_, _ = io.WriteString(conn, "250-fixture\r\n250 AUTH PLAIN\r\n")
					case strings.HasPrefix(line, "AUTH"):
						_, _ = io.WriteString(conn, "235 authenticated\r\n")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						_, _ = io.WriteString(conn, "250 accepted\r\n")
					case strings.HasPrefix(line, "DATA"):
						_, _ = io.WriteString(conn, "354 send data\r\n")
						for {
							v, e := reader.ReadString('\n')
							if e != nil {
								done <- e
								return
							}
							if v == ".\r\n" {
								break
							}
							body.WriteString(v)
						}
						received <- body.String()
						if !ack {
							done <- nil
							return
						}
						_, _ = io.WriteString(conn, "250 queued fixture-message-id\r\n")
					case strings.HasPrefix(line, "QUIT"):
						_, _ = io.WriteString(conn, "221 bye\r\n")
						done <- nil
						return
					default:
						_, _ = io.WriteString(conn, "500 unsupported\r\n")
					}
				}
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			n, _ := strconv.Atoi(port)
			transport := Transport{ValidateEndpoint: func(context.Context, string) error { return nil }, DialSMTP: func(ctx context.Context, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", address)
			}, TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
			result := transport.Send(t.Context(), ChannelSpec{Provider: "smtp"}, NotificationCredential{SMTPHost: host, SMTPPort: n, Security: "tls", Username: "fixture", Password: "fixture", From: "alerts@example.test", To: []string{"ops@example.test"}}, Message{Subject: "Argus告警", Text: "CPU 95%"})
			if result.Accepted != ack || result.Unknown == ack {
				t.Fatal(result)
			}
			select {
			case body := <-received:
				if !strings.Contains(body, "CPU 95%") || !strings.Contains(body, "To: ops@example.test") {
					t.Fatal("receiver body missing")
				}
				t.Log("SMTP receiver persisted body before DATA acknowledgement")
			case <-time.After(time.Second):
				t.Fatal("no received message")
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestProductionEndpointPolicyRejectsLoopbackAndPlaintext(t *testing.T) {
	for _, url := range []string{"http://127.0.0.1/hook", "https://127.0.0.1/hook", "https://169.254.169.254/hook"} {
		result := (Transport{}).Send(t.Context(), ChannelSpec{Provider: "wecom"}, NotificationCredential{WebhookURL: url}, Message{Text: "test"})
		if result.Accepted || result.ErrorCode != "NOTIFICATION_ENDPOINT_INVALID" {
			t.Fatal(url, result)
		}
	}
}
