# syntax=docker/dockerfile:1.7
#
# One image, one process, one file: the Go server with the built frontend
# embedded in it (see server/web/embed.go), on scratch.
#
#   docker build -t kickstand .
#   docker run --rm -p 8080:8080 -v kickstand-data:/data kickstand
#
# The final stage has no shell, no package manager and no libc — the binary is
# static because modernc.org/sqlite is pure Go, so CGO stays off. What scratch
# does not give us for free is arranged in the build stages below: a CA bundle
# for the outbound Nominatim/OSRM calls, and a writable /data owned by the
# unprivileged uid the server runs as.

# --- 1. the frontend --------------------------------------------------------
# node:22-alpine because Vite 7 wants node >=22.12.
FROM node:22-alpine AS web
WORKDIR /app

# Pinned rather than corepack-inferred: package.json has no packageManager
# field, and a build should not pick up whichever pnpm ships with the base image.
RUN npm install --global pnpm@10.15.0

# Dependencies first, so editing src/ does not reinstall them.
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY tsconfig.json vite.config.ts index.html ./
COPY src ./src
COPY public ./public
RUN pnpm build

# --- 2. the server ----------------------------------------------------------
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
# Where go:embed expects the app. This has to land before the build: `all:dist`
# needs at least one match to compile at all.
COPY --from=web /app/dist ./web/dist

# -trimpath keeps build paths out of the binary; -s -w drops the symbol table
# and DWARF, which is most of its size and none of its behaviour.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server .

# Directories the server needs at runtime, staged here because the final image
# has no shell to create them with. /tmp is for the temp files SQLite spills
# to; /data holds the database and is owned by the runtime uid.
RUN mkdir -p /out/data /out/tmp \
 && chown 10001:10001 /out/data \
 && chmod 1777 /out/tmp

# --- 3. what actually ships -------------------------------------------------
FROM scratch

# Nominatim and OSRM are reached over HTTPS; without this every lookup fails
# with "x509: certificate signed by unknown authority".
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=build --chown=10001:10001 /out/data /data
COPY --from=build /out/tmp /tmp
COPY --from=build /out/server /server

# Numeric, because there is no /etc/passwd to resolve a name against.
USER 10001:10001

ENV BIKETRIP_ADDR=:8080 \
    BIKETRIP_DB=/data/biketrip.db

# Mount something here to keep trips across `docker run --rm`. Without a fixed
# BIKETRIP_ADMIN_TOKEN the server prints a fresh one to stderr on every start.
VOLUME /data
EXPOSE 8080

# No HEALTHCHECK: scratch has nothing to run one with. Probe GET /api/health
# from outside the container instead.
ENTRYPOINT ["/server"]
