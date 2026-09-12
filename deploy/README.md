# Argus Kubernetes Deployment

`deploy/` contains the install contract, locked dependency versions, image builds and six Helm releases used by `argusctl`.

## Layout

```text
deploy/
├── docker/                 backend, web and patched MinIO images
├── helm/                   six Argus-owned Helm charts
├── profiles/               Evaluation and Production install configs
├── schemas/                ArgusInstallConfig v1alpha1 JSON Schema
└── versions.lock.yaml      tested build and middleware versions
```

The release order is:

```text
argus-foundation
argus-data-operators
argus-data
argus-sandbox
argus-platform
argus-telemetry-pipeline
```

Strimzi, Altinity and OpenSandbox upstream charts are installed by `argusctl` between the Argus-owned releases. Stage state is stored in `<release-id>-install-status` in the system namespace.

## Evaluation

Evaluation targets a disposable single-node cluster. It deploys one replica of every Argus runtime role, PostgreSQL, Redis, MinIO, Strimzi/Kafka, Altinity/ClickHouse, Keeper, OpenSandbox and the OTel ClickHouse writer. The three web applications continue to use the built-in mock API; the backend deployment validates process roles, configuration, health and lifecycle, not completed domain APIs.

Create a run-specific config from `profiles/evaluation.yaml`; do not reuse the default namespace names for concurrent runs. A normal local flow is:

```bash
go run ./cmd/argusctl preflight --config deploy/.cache/evaluation-<run-id>.yaml
go run ./cmd/argusctl plan --config deploy/.cache/evaluation-<run-id>.yaml
go run ./cmd/argusctl images build --config deploy/.cache/evaluation-<run-id>.yaml --platform linux/arm64
go run ./cmd/argusctl images load --config deploy/.cache/evaluation-<run-id>.yaml
go run ./cmd/argusctl install --config deploy/.cache/evaluation-<run-id>.yaml
go run ./cmd/argusctl status --config deploy/.cache/evaluation-<run-id>.yaml --output json
go run ./cmd/argusctl verify --config deploy/.cache/evaluation-<run-id>.yaml --output json
```

`images build` also cross-builds the Linux amd64/arm64 Connector distribution used by onboarding, then starts a run-owned local registry container. `images load` uses a privileged, run-labelled DaemonSet to import the exact images into the node containerd `k8s.io` namespace. Evaluation workloads use `imagePullPolicy: Never`.

During `install`, `argusctl` verifies the configured Collector release files and Ed25519 root, publishes Collector/Connector objects plus their install scripts to the bundled MinIO Artifact Store, and registers the exact Connector manifest after PostgreSQL migration. This is part of the normal installation transaction; running `argus-dev ... publish-artifacts` is only a development utility and is not required to make the product wizards work. For secured release automation the matching Connector signing private key may be supplied as `ARGUS_OTELCOL_SIGNING_PRIVATE_KEY`; repository development uses `deploy/.keys/otelcol-signing-key.json`.

Artifact probes, downloads, and Connector enrollment tunnel forwarding inside the cluster share one strict HTTPS route. `argusctl` discovers the ingress-nginx HTTPS Service while preserving each public URL, HTTP Host or TLS SNI, and CA validation. For multiple ingress services or another controller, set `spec.exposure.httpsInternalAddress` to the intended internal `host:port`. This deployment setting is not sent to external Hosts/Bastions and does not require changing their hosts files. Cluster workloads do not fall back to public DNS for this route, so a public host resolving to loopback in a Pod cannot redirect traffic into that same Pod.

### Windows 开发机本地安装补充

argusctl 支持 Windows（磁盘预检按平台分别实现），在 Docker Desktop（amd64 节点）上从 evaluation profile 起一次本地安装还需要：

1. 自行安装 ingress-nginx（`argusctl` 只安装 cert-manager，preflight 要求 IngressClass `nginx` 已存在）；
2. `images build` 使用 `--platform linux/amd64`；
3. profiles 里的 `spec.telemetry` 是占位值（sha256 为空串哈希、密钥 ID 与仓库密钥不一致），会触发 `artifact signing key does not match spec.telemetry signingKeyId/signingPublicKey`。先用 `make otelcol-distributions` 构建产物，再按 `deploy/.keys/otelcol-signing-key.json` 计算 sha256/size/Ed25519 签名回填 run 配置；URI 必须指向 `https://artifacts.<企业父域名>/argus-collector-artifacts/argus-otelcol/<version>/...`；
4. `argus-dev collector build` 依赖的 OC builder 在 Windows 上不会自动创建嵌套 dist 目录，先 `mkdir -p build/otelcol/dist/{linux-arm64,linux-amd64,windows-amd64}`；
5. `argusctl verify` 的 HTTPS/CORS 探测在 Windows Schannel 下使用 `--ssl-revoke-best-effort`，使 managed CA 和客户私有 CA 都能容忍缺少或不可达的 CRL/OCSP；显式 CA 链、主机名、有效期和已知吊销仍严格校验。参见 [curl 参数说明](https://curl.se/docs/manpage.html#--ssl-revoke-best-effort)。

For iterative local development after the first install, `make dev-upgrade` chains the light-weight update path in one step: `images build` → `images load` → rollout restart of every Argus deployment → wait for readiness. It skips the full `install` flow (Helm stages, secrets, migrations). The install config defaults to `deploy/.cache/argus-install-dev.yaml`; override with `DEV_CONFIG=...` (also `DEV_SYSTEM_NAMESPACE`, `DEV_OBSERVABILITY_NAMESPACE`, `KUBECTL` for another kube context).

Docker and local real frontend builds use the same `scripts/build-web.mjs real` entrypoint. Its API base defaults to `/`, so each portal sends `/api/v1/*` requests through its own HTTPS origin. Card and Platform links come from the Helm-provided `/argus-runtime.json`; explicit `VITE_*` overrides remain available for development. Every real build checks the resulting JavaScript and rejects mock seed markers or `.argus.invalid` placeholder endpoints before it can be packaged. `argus-dev web build --api-mode real` runs the same checks.

Chart or runtime configuration changes require `argusctl install` after image loading. Each install recreates the idempotent ClickHouse migration Job so an old Job's TTL cannot delete it between deployment readiness checks.

Portals are exposed through the ingress with mandatory TLS; map the install-config hosts to the ingress load-balancer address (for example in `/etc/hosts` on Docker Desktop):

- Enterprise (terminal WSS is same-origin: `wss://argus.dev/v1/sessions`): `https://argus.dev`
- Platform and first-time setup: `https://platform.argus.dev`
- Card Runtime (internal): `https://cards.argus.dev`
- Signed Connector/Collector downloads: `https://artifacts.argus.dev`
- Connector mTLS: `grpcs://connector.argus.dev:9443` (dedicated LoadBalancer service)

With `spec.pki.mode: managed`, trust the public CA from the versioned `argus-trust-bundle` before first browser access. Argus installers embed that Bundle for Connector/Collector use and do not modify the operating-system trust store. Browser trust is still an administrator-managed endpoint policy. Every origin and service has its own leaf certificate even though all leaves reference the same steady-state `ClusterIssuer`.

Host and manual Connector onboarding default to a one-line command. `spec.pki.bootstrapTLSMode` selects how that command downloads its initial dynamic script: managed/self-signed profiles default to `insecure-first-fetch`; an externally trusted certificate should use `strict`. The relaxed mode applies only to that first request and never activates as an automatic fallback. The downloaded script pins the versioned Argus Trust Bundle and strictly validates every later HTTPS request and installer digest. Because the first response can be replaced on an untrusted network, use `strict` whenever the target already trusts the serving certificate chain.

Host onboarding publishes immutable Linux amd64/arm64 and Windows amd64 Connector artifacts plus canonical POSIX/PowerShell installers. Ordinary Hosts install only Connector; Collector enablement is a later Host detail action. The Connector Gateway Pod includes `guacamole/guacd:1.6.0` bound to Pod loopback for Windows RDP sessions; RDP output is stored as encrypted `guacamole_v1` chunks and replayed as a visual desktop. Bastion Connectors choose two TLS passthrough listeners beginning at 8445 and 9445, advance within the reserved range when a port is occupied, and report the selected endpoints only after both listeners bind successfully.

Running a newly generated command or SSH install from another Argus deployment performs a transactional takeover. The new identity enrolls with staged key material before the live service, CA, marker, Collector, or Relay is changed. On success the old Connector is stopped and the new one becomes the only active local identity; on failure the current Connector remains runnable and the staged attempt can be resumed. See [Connector cross-cluster takeover](../docs/22-cross-cluster-connector-takeover.md).

Within one Argus enterprise, enrollment also fences a previous live Connector with the same stable instance ID only when its device fingerprint matches. This releases the uniqueness fence and moves the previous Host/Bastion projection offline in the same transaction; a different device fingerprint remains a hard conflict.

Inspect or rotate the Bundle with `argusctl pki status|rotate|extend|abort`; use `argusctl pki repair-command` only for a node that missed the complete overlap window. See [PKI and TLS design](../docs/18-pki-and-tls.md).

首次安装成功时，`argusctl install` 会在最终摘要中只显示一次包含 Setup Token Fragment 的 Platform 初始化链接。初始化者直接打开该链接，无需手工输入 Token；Platform 会立即从地址栏移除 Fragment，Token 只在当前页面内存中保留。链接遗失或过期时，在系统仍未初始化的前提下运行：

```bash
go run ./cmd/argusctl setup-token rotate --config deploy/.cache/evaluation-<run-id>.yaml
```

## Production Profile

`profiles/production.yaml` renders HA replicas, PDB, HPA, topology spread, two portal hosts and the isolated Card Runtime host. Production installation is intentionally blocked with:

- `POSTGRES_HA_ADR_REQUIRED`
- `SANDBOX_RUNTIME_ADR_REQUIRED`

The profile is available for schema validation, linting and rendering only. It must not be described as production-ready until both ADRs are resolved and the resulting topology passes HA, backup/restore and hardened sandbox validation.

## Cleanup

Evaluation cleanup is destructive and requires explicit confirmation:

```bash
go run ./cmd/argusctl uninstall \
  --config deploy/.cache/evaluation-<run-id>.yaml \
  --delete-data \
  --delete-owned-crds \
  --yes
```

Before removal, diagnostics are written to `artifacts/k8s-e2e/<release-id>/uninstall/`. Cleanup removes the run namespaces, run-owned releases and CRDs, loader DaemonSet, imported Argus images and local registry container. Production defaults retain data and shared cluster resources.

See [service and Kubernetes design](../docs/10-service-components-and-kubernetes-deployment.md), [current implementation status](../docs/13-current-implementation-and-kubernetes-rollout.md), and [PostgreSQL deployment decision](../docs/14-postgresql-deployment-decision.md).
