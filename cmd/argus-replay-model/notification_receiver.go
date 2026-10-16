//go:build m4e2e

package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var notificationRecords = struct {
	sync.Mutex
	rows     []map[string]any
	attempts map[string]int
}{rows: []map[string]any{}, attempts: map[string]int{}}

func notificationFaultAttempt(key string) int {
	notificationRecords.Lock()
	defer notificationRecords.Unlock()
	notificationRecords.attempts[key]++
	return notificationRecords.attempts[key]
}

func recordNotification(row map[string]any) error {
	notificationRecords.Lock()
	defer notificationRecords.Unlock()
	row["recorded_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	file, err := os.OpenFile("/tmp/plan6-notifications.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, _ := json.Marshal(row)
	if _, err = file.Write(append(raw, '\n')); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	notificationRecords.rows = append(notificationRecords.rows, row)
	return nil
}
func notificationReceiver(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		notificationRecords.Lock()
		defer notificationRecords.Unlock()
		writeJSON(w, 200, map[string]any{"items": notificationRecords.rows})
		return
	}
	provider := strings.TrimPrefix(r.URL.Path, "/fixtures/notifications/")
	var body map[string]any
	if r.Method != http.MethodPost || json.NewDecoder(io.LimitReader(r.Body, 65537)).Decode(&body) != nil {
		w.WriteHeader(400)
		return
	}
	valid := true
	const secret = "plan6-fixture-signing"
	if provider == "dingtalk" {
		stamp := r.URL.Query().Get("timestamp")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(stamp + "\n" + secret))
		valid = r.URL.Query().Get("sign") == base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	if provider == "feishu" {
		stamp, _ := body["timestamp"].(string)
		mac := hmac.New(sha256.New, []byte(stamp+"\n"+secret))
		valid = body["sign"] == base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	if !valid {
		writeJSON(w, 200, map[string]any{"errcode": 310000, "code": 19024})
		return
	}
	fault, key := r.URL.Query().Get("fault"), r.URL.Query().Get("case")
	if fault != "" && key != "" {
		attempt := notificationFaultAttempt(provider + ":" + key)
		failures, _ := strconv.Atoi(r.URL.Query().Get("failures"))
		if failures == 0 || attempt <= failures {
			if err := recordNotification(map[string]any{"provider": provider, "batch_id": r.Header.Get("X-Argus-Notification-Batch"), "case": key, "fault": fault, "attempt": attempt, "signature_valid": valid, "accepted": false}); err != nil {
				w.WriteHeader(503)
				return
			}
			switch fault {
			case "429":
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(429)
				return
			case "500":
				w.WriteHeader(500)
				return
			case "400":
				w.WriteHeader(400)
				return
			case "business":
				writeJSON(w, 200, map[string]any{"code": 19024, "errcode": 310000})
				return
			case "timeout":
				time.Sleep(20 * time.Second)
				return
			}
		}
	}
	row := map[string]any{"provider": provider, "body": body, "signature_valid": valid, "batch_id": r.Header.Get("X-Argus-Notification-Batch"), "case": key, "accepted": true}
	if err := recordNotification(row); err != nil {
		w.WriteHeader(503)
		return
	}
	if provider == "feishu" {
		writeJSON(w, 200, map[string]any{"code": 0})
	} else {
		writeJSON(w, 200, map[string]any{"errcode": 0})
	}
}
func runNotificationSMTP(certificate, key string) error {
	pair, err := tls.LoadX509KeyPair(certificate, key)
	if err != nil {
		return err
	}
	listener, err := tls.Listen("tcp", ":2525", &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
			reader := bufio.NewReader(conn)
			send := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
			send("220 plan6-receiver ESMTP")
			recipients := []string{}
			from := ""
			mode, faultCase := "", ""
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				switch {
				case strings.HasPrefix(line, "EHLO"):
					send("250-plan6-receiver")
					send("250 AUTH PLAIN")
				case strings.HasPrefix(line, "AUTH PLAIN"):
					encoded := strings.TrimSpace(strings.TrimPrefix(line, "AUTH PLAIN"))
					raw, err := base64.StdEncoding.DecodeString(encoded)
					parts := strings.Split(string(raw), "\x00")
					if err == nil && len(parts) == 3 && strings.HasPrefix(parts[1], "plan6-fault:") {
						fields := strings.SplitN(parts[1], ":", 3)
						if len(fields) == 3 {
							mode, faultCase = fields[1], fields[2]
						}
					}
					if err != nil || len(parts) != 3 || parts[2] != "plan6-fixture-password" || parts[1] != "plan6-fixture" && mode == "" {
						send("535 authentication rejected")
					} else {
						send("235 authenticated")
					}
					clear(raw)
				case strings.HasPrefix(line, "MAIL FROM:"):
					from = strings.TrimSpace(strings.TrimPrefix(line, "MAIL FROM:"))
					if mode == "temporary" || mode == "permanent" {
						attempt := notificationFaultAttempt("smtp:" + faultCase)
						if mode == "permanent" || attempt == 1 {
							_ = recordNotification(map[string]any{"provider": "smtp", "case": faultCase, "fault": mode, "attempt": attempt, "accepted": false})
							if mode == "temporary" {
								send("451 temporarily unavailable")
							} else {
								send("550 rejected permanently")
							}
							continue
						}
					}
					send("250 sender accepted")
				case strings.HasPrefix(line, "RCPT TO:"):
					recipients = append(recipients, strings.TrimSpace(strings.TrimPrefix(line, "RCPT TO:")))
					send("250 recipient accepted")
				case strings.HasPrefix(line, "DATA"):
					send("354 send body")
					var body strings.Builder
					for {
						v, e := reader.ReadString('\n')
						if e != nil {
							return
						}
						if v == ".\r\n" {
							break
						}
						body.WriteString(v)
					}
					row := map[string]any{"provider": "smtp", "from": from, "recipients": recipients, "body": body.String(), "queue_id": "p6-" + strconv.FormatInt(time.Now().UnixNano(), 10), "case": faultCase, "accepted": true}
					if recordNotification(row) != nil {
						send("451 persistence failed")
					} else if mode == "unknown" && notificationFaultAttempt("smtp:"+faultCase) == 1 {
						return // Durable receipt exists; no DATA acknowledgement reaches sender.
					} else if mode == "hold" && notificationFaultAttempt("smtp:"+faultCase) == 1 {
						time.Sleep(30 * time.Second)
						send("250 queued " + fmt.Sprint(row["queue_id"]))
					} else {
						send("250 queued " + fmt.Sprint(row["queue_id"]))
					}
				case strings.HasPrefix(line, "QUIT"):
					send("221 bye")
					return
				default:
					send("500 unsupported")
				}
			}
		}()
	}
}
