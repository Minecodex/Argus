package connector

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

const (
	defaultRelayHTTPSPort      = 8445
	defaultRelayGatewayPort    = 9445
	defaultRelayPortAttempts   = 20
	defaultRelayMaxConnections = 256
	defaultRelayRatePerMinute  = 120
	maxTLSClientHelloBytes     = 18 << 10
)

type bastionRelay struct {
	listeners                  []*relayListener
	reported                   []*connectorv1.BastionRelayListener
	generation                 uint64
	advertiseAddress           string
	errorCode                  string
	active, accepted, rejected atomic.Uint64
	ready                      atomic.Bool
	closeOnce                  sync.Once
}

type bastionRelayOptions struct {
	bindAddress      string
	httpsStartPort   int
	gatewayStartPort int
	portAttempts     int
	advertiseAddress string
}

type relayListener struct {
	kind        string
	listener    net.Listener
	upstreams   map[string]string
	allowedALPN map[string]bool
	semaphore   chan struct{}
	rate        *sourceRateLimiter
	parent      *bastionRelay
	logger      *slog.Logger
}

type sourceRateLimiter struct {
	mu      sync.Mutex
	window  time.Time
	limit   int
	counts  map[netip.Addr]int
	allowed []netip.Prefix
}

func startBastionRelay(ctx context.Context, store localStore, identity identityState, logger *slog.Logger) (*bastionRelay, error) {
	return startBastionRelayWithOptions(ctx, store, identity, logger, bastionRelayOptions{
		httpsStartPort: defaultRelayHTTPSPort, gatewayStartPort: defaultRelayGatewayPort, portAttempts: defaultRelayPortAttempts,
	})
}

func startBastionRelayWithOptions(ctx context.Context, store localStore, identity identityState, logger *slog.Logger, options bastionRelayOptions) (*bastionRelay, error) {
	if identity.Role != "bastion" {
		return nil, errors.New("bastion relay requires a Bastion Connector identity")
	}
	httpsUpstream, httpsSNI, err := relayEndpoint(identity.EnrollmentEndpoint, "https", os.Getenv("ARGUS_CONNECTOR_ENROLL_ADDRESS"))
	if err != nil {
		return nil, fmt.Errorf("configure Bastion HTTPS relay: %w", err)
	}
	gatewayUpstream, gatewaySNI, err := relayEndpoint(identity.GatewayEndpoint, "grpcs", os.Getenv("ARGUS_CONNECTOR_DIAL_ADDRESS"))
	if err != nil {
		return nil, fmt.Errorf("configure Bastion Gateway relay: %w", err)
	}
	artifactEndpoint := strings.TrimSpace(os.Getenv("ARGUS_BASTION_RELAY_ARTIFACT_ENDPOINT"))
	if artifactEndpoint == "" {
		artifactEndpoint = defaultArtifactEndpoint(identity.EnrollmentEndpoint)
	}
	artifactUpstream, artifactSNI, err := relayEndpoint(artifactEndpoint, "https", os.Getenv("ARGUS_CONNECTOR_ENROLL_ADDRESS"))
	if err != nil {
		return nil, fmt.Errorf("configure Bastion Artifact relay: %w", err)
	}
	maxConnections := positiveEnvironmentInt("ARGUS_BASTION_RELAY_MAX_CONNECTIONS", defaultRelayMaxConnections)
	ratePerMinute := positiveEnvironmentInt("ARGUS_BASTION_RELAY_RATE_PER_MINUTE", defaultRelayRatePerMinute)
	allowed, err := relaySourcePrefixes(os.Getenv("ARGUS_BASTION_RELAY_SOURCE_CIDRS"))
	if err != nil {
		return nil, err
	}
	state, persisted, err := store.loadRelayPortState()
	if err != nil {
		return nil, fmt.Errorf("load Bastion relay state: %w", err)
	}
	if options.portAttempts < 1 {
		options.portAttempts = defaultRelayPortAttempts
	}
	if options.httpsStartPort < 1 {
		options.httpsStartPort = defaultRelayHTTPSPort
	}
	if options.gatewayStartPort < 1 {
		options.gatewayStartPort = defaultRelayGatewayPort
	}
	if persisted {
		options.httpsStartPort, options.gatewayStartPort, options.portAttempts = int(state.HTTPSPort), int(state.GatewayPort), 1
	} else {
		state.Generation = 1
		state.AdvertiseAddress = strings.TrimSpace(options.advertiseAddress)
		if state.AdvertiseAddress == "" {
			state.AdvertiseAddress = discoverRelayAdvertiseAddress()
		}
	}
	relay := &bastionRelay{advertiseAddress: state.AdvertiseAddress}
	if persisted {
		relay.generation = state.Generation
	}
	definitions := []struct {
		kind      string
		startPort int
		upstreams map[string]string
		alpn      map[string]bool
	}{
		{"https", options.httpsStartPort, map[string]string{strings.ToLower(httpsSNI): httpsUpstream, strings.ToLower(artifactSNI): artifactUpstream}, map[string]bool{"h2": true, "http/1.1": true}},
		{"gateway", options.gatewayStartPort, map[string]string{strings.ToLower(gatewaySNI): gatewayUpstream}, map[string]bool{"h2": true}},
	}
	bindFailed := false
	for _, definition := range definitions {
		listener, report, listenErr := bindBastionRelayListener(definition.kind, options.bindAddress, definition.startPort, options.portAttempts)
		relay.reported = append(relay.reported, report)
		if listenErr != nil {
			bindFailed = true
			if persisted {
				report.ErrorCode = "RELAY_PORT_IN_USE"
			}
			if relay.errorCode == "" {
				relay.errorCode = report.GetErrorCode()
			}
			continue
		}
		entry := &relayListener{kind: definition.kind, listener: listener, upstreams: definition.upstreams,
			allowedALPN: definition.alpn, semaphore: make(chan struct{}, maxConnections),
			rate: &sourceRateLimiter{window: time.Now().UTC(), limit: ratePerMinute, counts: map[netip.Addr]int{}, allowed: allowed}, parent: relay, logger: logger}
		relay.listeners = append(relay.listeners, entry)
	}
	if bindFailed {
		relay.Close()
		return relay, nil
	}
	state.HTTPSPort, state.GatewayPort = relay.reported[0].GetPort(), relay.reported[1].GetPort()
	if !validRelayAdvertiseAddress(state.AdvertiseAddress) {
		relay.errorCode = "RELAY_ADDRESS_UNAVAILABLE"
		relay.Close()
		return relay, nil
	}
	if !persisted {
		if err = store.saveRelayPortState(state); err != nil {
			relay.errorCode = "RELAY_STATE_PERSIST_FAILED"
			relay.Close()
			return relay, nil
		}
		relay.generation = state.Generation
	}
	for _, listener := range relay.listeners {
		go listener.run(ctx)
	}
	relay.ready.Store(true)
	return relay, nil
}

func bindBastionRelayListener(kind, bindAddress string, startPort, attempts int) (net.Listener, *connectorv1.BastionRelayListener, error) {
	report := &connectorv1.BastionRelayListener{Kind: kind, BindAddress: bindAddress, Port: uint32(startPort), Status: "port_conflict", ErrorCode: "RELAY_PORT_RANGE_EXHAUSTED"}
	for offset := 0; offset < attempts && startPort+offset <= 65535; offset++ {
		port := startPort + offset
		address := net.JoinHostPort(bindAddress, strconv.Itoa(port))
		listener, err := net.Listen("tcp", address)
		if err == nil {
			report.Port, report.Status, report.ErrorCode = uint32(port), "ready", ""
			report.BindAddress = listener.Addr().String()
			return listener, report, nil
		}
		report.Port = uint32(port)
	}
	return nil, report, errors.New("Bastion relay port range is unavailable")
}

func (relay *bastionRelay) Close() {
	if relay == nil {
		return
	}
	relay.closeOnce.Do(func() {
		relay.ready.Store(false)
		for _, listener := range relay.listeners {
			_ = listener.listener.Close()
		}
	})
}

func (relay *bastionRelay) snapshot() *connectorv1.BastionRelayStatus {
	if relay == nil {
		return nil
	}
	status := "degraded"
	if relay.ready.Load() {
		status = "ready"
	}
	listeners := make([]*connectorv1.BastionRelayListener, 0, len(relay.reported))
	for _, listener := range relay.reported {
		copy := &connectorv1.BastionRelayListener{Kind: listener.GetKind(), BindAddress: listener.GetBindAddress(), Port: listener.GetPort(),
			Status: listener.GetStatus(), ErrorCode: listener.GetErrorCode()}
		if !relay.ready.Load() && copy.GetStatus() == "ready" {
			copy.Status = "closed"
		}
		listeners = append(listeners, copy)
	}
	return &connectorv1.BastionRelayStatus{Status: status, Listeners: listeners,
		ActiveConnections: uint32(min(relay.active.Load(), uint64(^uint32(0)))), AcceptedConnections: relay.accepted.Load(), RejectedConnections: relay.rejected.Load(),
		Generation: relay.generation, AdvertiseAddress: relay.advertiseAddress, ErrorCode: relay.errorCode}
}

func discoverRelayAdvertiseAddress() string {
	if connection, err := net.DialTimeout("udp", "192.0.2.1:9", time.Second); err == nil {
		if local, ok := connection.LocalAddr().(*net.UDPAddr); ok && local.IP != nil && !local.IP.IsLoopback() && !local.IP.IsUnspecified() {
			_ = connection.Close()
			return local.IP.String()
		}
		_ = connection.Close()
	}
	interfaces, err := net.Interfaces()
	if err == nil {
		type candidate struct {
			score int
			value string
		}
		values := make([]candidate, 0)
		for _, networkInterface := range interfaces {
			if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addresses, addressErr := networkInterface.Addrs()
			if addressErr != nil {
				continue
			}
			for _, raw := range addresses {
				prefix, prefixErr := netip.ParsePrefix(raw.String())
				if prefixErr != nil {
					continue
				}
				address := prefix.Addr().Unmap()
				if !address.IsValid() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
					continue
				}
				score := 3
				if address.Is4() && address.IsPrivate() {
					score = 0
				} else if address.Is4() {
					score = 1
				} else if address.IsPrivate() {
					score = 2
				}
				values = append(values, candidate{score: score, value: address.String()})
			}
		}
		sort.Slice(values, func(i, j int) bool {
			if values[i].score == values[j].score {
				return values[i].value < values[j].value
			}
			return values[i].score < values[j].score
		})
		if len(values) > 0 {
			return values[0].value
		}
	}
	value := hostname()
	if validRelayAdvertiseAddress(value) {
		return value
	}
	return ""
}

func validRelayAdvertiseAddress(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "/\\@?#") {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character != '-' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
				return false
			}
		}
	}
	return true
}

func (listener *relayListener) run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = listener.listener.Close()
	}()
	for {
		connection, err := listener.listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				listener.parent.ready.Store(false)
				listener.logger.Error("Bastion relay listener stopped", "kind", listener.kind, "error", err)
			}
			return
		}
		select {
		case listener.semaphore <- struct{}{}:
			go func() {
				defer func() { <-listener.semaphore }()
				listener.handle(ctx, connection)
			}()
		default:
			listener.parent.rejected.Add(1)
			_ = connection.Close()
		}
	}
}

func (listener *relayListener) handle(ctx context.Context, downstream net.Conn) {
	defer downstream.Close()
	address, ok := remoteAddress(downstream.RemoteAddr())
	if !ok || !listener.rate.accept(address, time.Now().UTC()) {
		listener.parent.rejected.Add(1)
		return
	}
	_ = downstream.SetReadDeadline(time.Now().Add(10 * time.Second))
	initial, sni, alpn, err := readTLSClientHello(downstream)
	upstreamAddress, allowedSNI := listener.upstreams[strings.ToLower(sni)]
	if err != nil || !allowedSNI || !containsAllowedALPN(alpn, listener.allowedALPN) {
		listener.parent.rejected.Add(1)
		return
	}
	_ = downstream.SetReadDeadline(time.Time{})
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	upstream, err := dialer.DialContext(ctx, "tcp", upstreamAddress)
	if err != nil {
		listener.parent.rejected.Add(1)
		return
	}
	defer upstream.Close()
	if _, err = upstream.Write(initial); err != nil {
		listener.parent.rejected.Add(1)
		return
	}
	listener.parent.accepted.Add(1)
	listener.parent.active.Add(1)
	defer listener.parent.active.Add(^uint64(0))
	done := make(chan struct{}, 2)
	go copyRelay(upstream, downstream, done)
	go copyRelay(downstream, upstream, done)
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func copyRelay(destination, source net.Conn, done chan<- struct{}) {
	_, _ = io.Copy(destination, source)
	if closer, ok := destination.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
	done <- struct{}{}
}

func relayEndpoint(raw, expectedScheme, dialOverride string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != expectedScheme || parsed.Hostname() == "" || parsed.User != nil {
		return "", "", errors.New("relay endpoint is invalid")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	upstream := net.JoinHostPort(parsed.Hostname(), port)
	if dialOverride != "" {
		if _, _, err = net.SplitHostPort(dialOverride); err != nil {
			return "", "", errors.New("relay dial override is invalid")
		}
		upstream = dialOverride
	}
	return upstream, parsed.Hostname(), nil
}

func defaultArtifactEndpoint(enrollmentEndpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(enrollmentEndpoint))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	labels := strings.Split(parsed.Hostname(), ".")
	parent := parsed.Hostname()
	if len(labels) >= 3 {
		parent = strings.Join(labels[1:], ".")
	}
	return "https://artifacts." + parent
}

func relaySourcePrefixes(raw string) ([]netip.Prefix, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8,fc00::/7,::1/128"
	}
	values := strings.Split(raw, ",")
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid Bastion relay source CIDR %q", value)
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func (limiter *sourceRateLimiter) accept(address netip.Addr, now time.Time) bool {
	allowed := false
	for _, prefix := range limiter.allowed {
		if prefix.Contains(address) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if now.Sub(limiter.window) >= time.Minute {
		limiter.window, limiter.counts = now, map[netip.Addr]int{}
	}
	limiter.counts[address]++
	return limiter.counts[address] <= limiter.limit
}

func remoteAddress(value net.Addr) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(value.String())
	if err != nil {
		return netip.Addr{}, false
	}
	address, err := netip.ParseAddr(host)
	return address.Unmap(), err == nil
}

func containsAllowedALPN(values []string, allowed map[string]bool) bool {
	for _, value := range values {
		if allowed[value] {
			return true
		}
	}
	return false
}

func readTLSClientHello(reader io.Reader) ([]byte, string, []string, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 22 {
		return nil, "", nil, errors.New("TLS ClientHello record is missing")
	}
	length := int(binary.BigEndian.Uint16(header[3:5]))
	if length < 4 || length > maxTLSClientHelloBytes-5 {
		return nil, "", nil, errors.New("TLS ClientHello record length is invalid")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil || payload[0] != 1 {
		return nil, "", nil, errors.New("TLS ClientHello payload is invalid")
	}
	handshakeLength := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if handshakeLength+4 > len(payload) {
		return nil, "", nil, errors.New("fragmented TLS ClientHello is unsupported")
	}
	sni, alpn, err := parseClientHelloExtensions(payload[4 : handshakeLength+4])
	return append(header, payload...), sni, alpn, err
}

func parseClientHelloExtensions(body []byte) (string, []string, error) {
	if len(body) < 35 {
		return "", nil, errors.New("TLS ClientHello is truncated")
	}
	offset := 34
	if offset >= len(body) || offset+1+int(body[offset]) > len(body) {
		return "", nil, errors.New("TLS session id is invalid")
	}
	offset += 1 + int(body[offset])
	if offset+2 > len(body) {
		return "", nil, errors.New("TLS cipher suites are missing")
	}
	cipherLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2 + cipherLength
	if offset >= len(body) || offset+1+int(body[offset]) > len(body) {
		return "", nil, errors.New("TLS compression methods are invalid")
	}
	offset += 1 + int(body[offset])
	if offset+2 > len(body) {
		return "", nil, errors.New("TLS extensions are missing")
	}
	extensionLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if offset+extensionLength > len(body) {
		return "", nil, errors.New("TLS extensions are truncated")
	}
	end := offset + extensionLength
	var sni string
	var alpn []string
	for offset+4 <= end {
		typeCode := binary.BigEndian.Uint16(body[offset : offset+2])
		valueLength := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		offset += 4
		if offset+valueLength > end {
			return "", nil, errors.New("TLS extension is truncated")
		}
		value := body[offset : offset+valueLength]
		offset += valueLength
		switch typeCode {
		case 0:
			if len(value) >= 5 && value[2] == 0 {
				nameLength := int(binary.BigEndian.Uint16(value[3:5]))
				if 5+nameLength <= len(value) {
					sni = string(value[5 : 5+nameLength])
				}
			}
		case 16:
			if len(value) < 2 || int(binary.BigEndian.Uint16(value[:2]))+2 != len(value) {
				return "", nil, errors.New("TLS ALPN extension is invalid")
			}
			for index := 2; index < len(value); {
				itemLength := int(value[index])
				index++
				if itemLength == 0 || index+itemLength > len(value) {
					return "", nil, errors.New("TLS ALPN value is invalid")
				}
				alpn = append(alpn, string(value[index:index+itemLength]))
				index += itemLength
			}
		}
	}
	if sni == "" || len(alpn) == 0 {
		return "", nil, errors.New("TLS SNI or ALPN is missing")
	}
	return sni, alpn, nil
}

func positiveEnvironmentInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
