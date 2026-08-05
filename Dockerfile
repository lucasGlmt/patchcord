# Multi-stage build (ADR-0039): a CGO-free binary (modernc.org/sqlite is
# pure Go — no libc needed at runtime) copied into a minimal, shell-less
# base. The core still never bundles a concrete business integration
# (non-negotiable #3): mysql/postgresql/openai are not baked in here —
# install one at runtime via a volume, see docker-compose.yml. The five
# generic reference plugins (text, json, encoding, http, time) are, as
# separate supervised processes embedded in the binary itself, exactly
# like a native `patchcord` install — see ADR-0059.
FROM golang:1.25 AS build
WORKDIR /src

# Passed by `make docker-build` (from `git describe`/`git rev-parse`) or by
# the release workflow (from the pushed tag) — .dockerignore excludes .git,
# so the build stage cannot derive these itself. Unset, they fall back to
# internal/version's own "dev"/"none"/"unknown" defaults.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Builds patchcord's embedded reference plugins (ADR-0059) for this image's
# own GOOS/GOARCH before compiling the agent — go:embed reads whatever's on
# disk under internal/plugins/embedded/bin/<goos>_<goarch>/ at compile
# time. Mirrors `make build-embedded-plugins` without depending on `make`,
# which the base golang image doesn't include.
RUN set -eux; \
	goos="$(go env GOOS)"; \
	goarch="$(go env GOARCH)"; \
	dir="internal/plugins/embedded/bin/${goos}_${goarch}"; \
	mkdir -p "$dir"; \
	for name in text json encoding http time; do \
		go build -o "$dir/$name" "./plugins/examples/$name"; \
	done

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w \
	-X github.com/lucasglmt/patchcord/internal/version.Version=${VERSION} \
	-X github.com/lucasglmt/patchcord/internal/version.Commit=${COMMIT} \
	-X github.com/lucasglmt/patchcord/internal/version.Date=${DATE}" \
	-o /out/patchcord ./cmd/patchcord

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
