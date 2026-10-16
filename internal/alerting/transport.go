package alerting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kakj-go/Argus/internal/integration/remotemcp"
)

// NotificationCredential is private broker data. It is never an API DTO, a
// model result, a pending-action preview, or a rendered notification variable.
type NotificationCredential struct {
	WebhookURL    string   `json:"webhook_url"`
	SigningSecret string   `json:"signing_secret"`
	SMTPHost      string   `json:"smtp_host"`
	SMTPPort      int      `json:"smtp_port"`
	Security      string   `json:"security"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	From          string   `json:"from"`
	To            []string `json:"to"`
}
type Transport struct {
	Policy           remotemcp.EndpointPolicy
	Client           *http.Client
	ValidateEndpoint func(context.Context, string) error
	DialSMTP         func(context.Context, string) (net.Conn, error)
	TLSConfig        *tls.Config
	Now              func() time.Time
}
type SendResult struct {
	Canceled   bool
	Accepted   bool
	Code       string
	ErrorCode  string
	Retryable  bool
	Unknown    bool
	RetryAfter time.Duration
}

func (t Transport) Send(ctx context.Context, ch ChannelSpec, c NotificationCredential, m Message) SendResult {
	if ch.Provider == "smtp" {
		return t.sendSMTP(ctx, c, m)
	}
	return t.sendWebhook(ctx, ch, c, m)
}
func (t Transport) sendWebhook(ctx context.Context, ch ChannelSpec, c NotificationCredential, m Message) SendResult {
	invalid := SendResult{ErrorCode: "NOTIFICATION_ENDPOINT_INVALID"}
	validate := t.ValidateEndpoint
	if validate == nil {
		validate = func(ctx context.Context, raw string) error { _, err := t.Policy.Validate(ctx, raw); return err }
	}
	if validate(ctx, c.WebhookURL) != nil {
		return invalid
	}
	u, err := url.Parse(c.WebhookURL)
	if err != nil || u.Scheme != "https" {
		return invalid
	}
	now := time.Now().UTC()
	if t.Now != nil {
		now = t.Now().UTC()
	}
	text := m.Text
	body := map[string]any{}
	switch ch.Provider {
	case "dingtalk":
		body = map[string]any{"msgtype": "text", "text": map[string]any{"content": text}, "at": map[string]any{"atUserIds": ch.MentionIDs, "isAtAll": ch.MentionAll}}
		if c.SigningSecret != "" {
			stamp := strconv.FormatInt(now.UnixMilli(), 10)
			mac := hmac.New(sha256.New, []byte(c.SigningSecret))
			_, _ = mac.Write([]byte(stamp + "\n" + c.SigningSecret))
			query := u.Query()
			query.Set("timestamp", stamp)
			query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
			u.RawQuery = query.Encode()
		}
	case "wecom":
		mentions := append([]string{}, ch.MentionIDs...)
		if ch.MentionAll {
			mentions = append(mentions, "@all")
		}
		body = map[string]any{"msgtype": "text", "text": map[string]any{"content": text, "mentioned_list": mentions}}
	case "feishu":
		for _, id := range ch.MentionIDs {
			text += "\n<at user_id=\"" + id + "\"></at>"
		}
		if ch.MentionAll {
			text += "\n<at user_id=\"all\"></at>"
		}
		body = map[string]any{"msg_type": "text", "content": map[string]any{"text": text}}
		if c.SigningSecret != "" {
			stamp := strconv.FormatInt(now.Unix(), 10)
			mac := hmac.New(sha256.New, []byte(stamp+"\n"+c.SigningSecret))
			body["timestamp"] = stamp
			body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
	default:
		return SendResult{ErrorCode: "NOTIFICATION_PROVIDER_INVALID"}
	}
	encoded, err := json.Marshal(body)
	if err != nil || len(encoded) > 64<<10 {
		return SendResult{ErrorCode: "NOTIFICATION_MESSAGE_INVALID"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(encoded))
	if err != nil {
		return invalid
	}
	request.Header.Set("Content-Type", "application/json")
	if m.BatchID != [16]byte{} {
		request.Header.Set("X-Argus-Notification-Batch", m.BatchID.String())
	}
	var wrote atomic.Bool
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			wrote.Store(true)
		}
	}}))
	client := t.Client
	if client == nil {
		client = t.Policy.Client()
	}
	response, err := client.Do(request)
	if err != nil {
		if !wrote.Load() {
			return SendResult{ErrorCode: "NOTIFICATION_TRANSPORT_UNAVAILABLE", Retryable: true}
		}
		return SendResult{ErrorCode: "NOTIFICATION_TRANSPORT_UNKNOWN", Unknown: true}
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		delay := time.Minute
		if n, e := strconv.Atoi(response.Header.Get("Retry-After")); e == nil && n > 0 {
			delay = time.Duration(n) * time.Second
		}
		return SendResult{Code: "429", ErrorCode: "NOTIFICATION_RATE_LIMITED", Retryable: true, RetryAfter: delay}
	}
	if response.StatusCode >= 500 {
		return SendResult{Code: fmt.Sprint(response.StatusCode), ErrorCode: "NOTIFICATION_UPSTREAM_UNAVAILABLE", Retryable: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SendResult{Code: fmt.Sprint(response.StatusCode), ErrorCode: "NOTIFICATION_HTTP_REJECTED"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(data) > 16384 {
		return SendResult{ErrorCode: "NOTIFICATION_RESPONSE_UNKNOWN", Unknown: true}
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(data, &result) != nil {
		return SendResult{ErrorCode: "NOTIFICATION_RESPONSE_UNKNOWN", Unknown: true}
	}
	key := "errcode"
	if ch.Provider == "feishu" {
		key = "code"
		if _, ok := result[key]; !ok {
			key = "StatusCode"
		}
	}
	raw, ok := result[key]
	if !ok {
		return SendResult{ErrorCode: "NOTIFICATION_RESPONSE_UNKNOWN", Unknown: true}
	}
	var code int64
	if json.Unmarshal(raw, &code) != nil {
		return SendResult{ErrorCode: "NOTIFICATION_RESPONSE_UNKNOWN", Unknown: true}
	}
	if code != 0 {
		return SendResult{Code: strconv.FormatInt(code, 10), ErrorCode: "NOTIFICATION_PROVIDER_REJECTED", Retryable: code == 45009 || code == 130101 || code == 9499}
	}
	return SendResult{Accepted: true, Code: "0"}
}
func (t Transport) sendSMTP(ctx context.Context, c NotificationCredential, m Message) SendResult {
	invalid := SendResult{ErrorCode: "NOTIFICATION_SMTP_INVALID"}
	if c.SMTPHost == "" || c.SMTPPort < 1 || c.SMTPPort > 65535 || len(c.To) == 0 || len(c.To) > 100 || (c.Security != "tls" && c.Security != "starttls") {
		return invalid
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil || strings.ContainsAny(m.Subject, "\r\n") {
		return invalid
	}
	recipients := []string{}
	for _, value := range c.To {
		address, err := mail.ParseAddress(value)
		if err != nil {
			return invalid
		}
		recipients = append(recipients, address.Address)
	}
	endpoint := "https://" + net.JoinHostPort(c.SMTPHost, strconv.Itoa(c.SMTPPort))
	validate := t.ValidateEndpoint
	if validate == nil {
		validate = func(ctx context.Context, raw string) error { _, err := t.Policy.Validate(ctx, raw); return err }
	}
	if validate(ctx, endpoint) != nil {
		return SendResult{ErrorCode: "NOTIFICATION_ENDPOINT_INVALID"}
	}
	dial := t.DialSMTP
	if dial == nil {
		dial = func(ctx context.Context, address string) (net.Conn, error) {
			client := t.Policy.Client()
			transport := client.Transport.(*http.Transport)
			return transport.DialContext(ctx, "tcp", address)
		}
	}
	conn, err := dial(ctx, net.JoinHostPort(c.SMTPHost, strconv.Itoa(c.SMTPPort)))
	if err != nil {
		return SendResult{ErrorCode: "NOTIFICATION_SMTP_UNAVAILABLE", Retryable: true}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	tlsConfig := &tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12}
	if clientTLS := t.Policy.Client().Transport.(*http.Transport).TLSClientConfig; clientTLS != nil {
		tlsConfig = clientTLS.Clone()
		tlsConfig.ServerName = c.SMTPHost
	}
	if t.TLSConfig != nil {
		tlsConfig = t.TLSConfig.Clone()
		tlsConfig.ServerName = c.SMTPHost
	}
	if c.Security == "tls" {
		secured := tls.Client(conn, tlsConfig)
		if secured.HandshakeContext(ctx) != nil {
			return SendResult{ErrorCode: "NOTIFICATION_SMTP_TLS_FAILED"}
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, c.SMTPHost)
	if err != nil {
		return SendResult{ErrorCode: "NOTIFICATION_SMTP_UNAVAILABLE", Retryable: true}
	}
	defer client.Close()
	if c.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return SendResult{ErrorCode: "NOTIFICATION_SMTP_TLS_REQUIRED"}
		}
		if client.StartTLS(tlsConfig) != nil {
			return SendResult{ErrorCode: "NOTIFICATION_SMTP_TLS_FAILED"}
		}
	}
	if c.Username != "" {
		if client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.SMTPHost)) != nil {
			return SendResult{ErrorCode: "NOTIFICATION_SMTP_AUTH_FAILED"}
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return smtpFailure(err, "NOTIFICATION_SMTP_SENDER_REJECTED", false)
	}
	for _, address := range recipients {
		if err := client.Rcpt(address); err != nil {
			return smtpFailure(err, "NOTIFICATION_SMTP_RECIPIENT_REJECTED", false)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpFailure(err, "NOTIFICATION_SMTP_DATA_REJECTED", false)
	}
	body := "From: " + from.String() + "\r\nTo: " + strings.Join(recipients, ", ") + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n", "\r\n") + "\r\n"
	if m.BatchID != [16]byte{} {
		body = "Message-ID: <" + m.BatchID.String() + "@" + strings.SplitN(from.Address, "@", 2)[1] + ">\r\nX-Argus-Notification-Batch: " + m.BatchID.String() + "\r\n" + body
	}
	if _, err = io.WriteString(writer, body); err != nil {
		return SendResult{ErrorCode: "NOTIFICATION_SMTP_RESULT_UNKNOWN", Unknown: true}
	}
	if err = writer.Close(); err != nil {
		return smtpFailure(err, "NOTIFICATION_SMTP_DATA_REJECTED", true)
	}
	_ = client.Quit()
	return SendResult{Accepted: true, Code: "250"}
}

func smtpFailure(err error, code string, afterData bool) SendResult {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return SendResult{Code: strconv.Itoa(reply.Code), ErrorCode: code, Retryable: reply.Code >= 400 && reply.Code < 500}
	}
	if afterData {
		return SendResult{ErrorCode: "NOTIFICATION_SMTP_RESULT_UNKNOWN", Unknown: true}
	}
	return SendResult{ErrorCode: code, Retryable: true}
}
