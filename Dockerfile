# Multi-stage build (ADR-0039): a CGO-free binary (modernc.org/sqlite is
# pure Go — no libc needed at runtime) copied into a minimal, shell-less
# base. No plugin binaries are baked in here — the core never bundles a
# concrete business integration (non-negotiable #3); install one at
# runtime via a volume, see docker-compose.yml.
FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/patchcord ./cmd/patchcord

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/patchcord /usr/local/bin/patchcord
# A container-appropriate default config (0.0.0.0, not the CLI's own
# 127.0.0.1-only default — see ADR-0039's Context) — override by mounting a
# replacement over this path, or via PATCHCORD_LISTEN/PATCHCORD_DATA_DIR/
# --listen/--data-dir, all of which still take precedence (ADR-0038).
COPY docker/config.yaml /etc/patchcord/config.yaml

EXPOSE 7331
ENTRYPOINT ["patchcord"]
CMD ["serve", "--config", "/etc/patchcord/config.yaml"]
