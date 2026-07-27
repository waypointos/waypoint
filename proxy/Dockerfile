# Build context is the repo root so we can include both dashboard/ and proxy/.
# Build: docker build -t waypoint-proxy:dev -f proxy/Dockerfile .

# Stage 1 — build the dashboard
FROM node:20-alpine AS dashboard
# pnpm 10 prompts before purging node_modules; CI=true makes it
# non-interactive (the .dockerignore should already keep node_modules
# out of the context, but this is belt-and-suspenders for safety).
ENV CI=true
WORKDIR /src
RUN corepack enable
# The dashboard is part of a pnpm workspace rooted at the repo
# (pnpm-workspace.yaml includes `dashboard` and `dashboard/packages/*`),
# so the install must run at /src with the root lockfile and have every
# workspace package present for `workspace:*` deps to resolve.
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY dashboard ./dashboard
COPY protocol ./protocol
RUN pnpm install --frozen-lockfile
RUN pnpm --filter dashboard build

# Stage 2 — build the proxy with the dashboard embedded
FROM golang:1.25-alpine AS proxy
WORKDIR /src
COPY proxy ./proxy
# proxy/go.mod replaces two intra-repo modules with sibling paths relative to
# /src/proxy, so both source trees must be present in the build context:
#   protocol/gen/go => ../protocol/gen/go   (generated bindings)
#   modverify        => ../modverify         (shared cosign verifier)
COPY protocol ./protocol
COPY modverify ./modverify
COPY --from=dashboard /src/dashboard/dist /src/proxy/web/dist
WORKDIR /src/proxy
RUN go build -trimpath -ldflags="-s -w" -o /out/waypoint-proxy ./cmd/waypoint-proxy

# Stage 3 — runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=proxy /out/waypoint-proxy /usr/local/bin/waypoint-proxy
EXPOSE 8081
ENV PORT=8081
CMD ["/usr/local/bin/waypoint-proxy"]
