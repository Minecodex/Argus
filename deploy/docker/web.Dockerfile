# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM mirror.gcr.io/library/node:24.19.0-alpine@sha256:d32cdf619f63fe0471182d08996dd516c6275bb5fd31ae06e55a570bd9e1ad43 AS build

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
WORKDIR /src

RUN corepack enable && corepack prepare pnpm@11.21.0 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.base.json ./
COPY web/apps/enterprise/package.json ./web/apps/enterprise/
COPY web/apps/platform/package.json ./web/apps/platform/
COPY web/apps/template-runtime/package.json ./web/apps/template-runtime/
COPY web/packages/api-client/package.json ./web/packages/api-client/
COPY web/packages/auth/package.json ./web/packages/auth/
COPY web/packages/design-tokens/package.json ./web/packages/design-tokens/
COPY web/packages/observability/package.json ./web/packages/observability/
COPY web/packages/ui/package.json ./web/packages/ui/
RUN --mount=type=cache,id=argus-pnpm-store,target=/pnpm/store \
    pnpm install --store-dir /pnpm/store --frozen-lockfile --fetch-retries 5 --fetch-timeout 300000
# Keep dependency installation cached when application source changes.
COPY scripts ./scripts/
COPY web ./web
# The query editor compiles the same read-only SDL as the Go trace engine.
COPY internal/telemetry/queryengine/skywalking/schema/trace.graphql ./internal/telemetry/queryengine/skywalking/schema/trace.graphql
ARG VITE_API_MODE=real
ARG VITE_API_BASE_URL=/
# Template origin and platform login URL are resolved at runtime from
# /argus-runtime.json (injected by Helm); VITE_* values are dev/mock overrides.
ARG VITE_TEMPLATE_ORIGIN=
ARG VITE_PLATFORM_URL=
ARG VITE_DIRECT_EGRESS_ADDRESSES=
RUN VITE_API_MODE=$VITE_API_MODE \
    VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_TEMPLATE_ORIGIN=$VITE_TEMPLATE_ORIGIN \
    VITE_PLATFORM_URL=$VITE_PLATFORM_URL \
    VITE_DIRECT_EGRESS_ADDRESSES=$VITE_DIRECT_EGRESS_ADDRESSES \
    node scripts/build-web.mjs "$VITE_API_MODE"

FROM mirror.gcr.io/nginxinc/nginx-unprivileged:1.29.4-alpine@sha256:a6c4f61f456b85b8fdf7ec7ab28cc3e299440e6fb4a9dea520e5fd8fd440025e
COPY deploy/docker/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /src/web/apps/enterprise/dist /srv/enterprise
COPY --from=build /src/web/apps/platform/dist /srv/platform
COPY --from=build /src/web/apps/template-runtime/dist /srv/template-runtime
EXPOSE 8080 8081 8083
