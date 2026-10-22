FROM gotenberg/gotenberg:8.38.1-chromium@sha256:5da4ab7b8fe7326537369e17b661be765d9a2887f77722faacdbe6d55812fb96 AS gotenberg-base

FROM gotenberg-base AS apt-patch
USER root
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends --only-upgrade \
        chromium chromium-common libexpat1; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

# build gotenberg binary with patched Go (heavy handed possibly remove)
FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4 AS go-patch
ENV CGO_ENABLED=0
ADD https://github.com/gotenberg/gotenberg.git#e4ee0f29d648f8d5a0a766dd797d108361905deb /src
WORKDIR /src
RUN go mod download && go mod verify
RUN go build -o /out/gotenberg-chromium \
    -ldflags "-s -w -X 'github.com/gotenberg/gotenberg/v8/cmd.Version=8.38.1'" \
    cmd/gotenberg-chromium/main.go

# base image already has non-root "gotenberg" user (uid/gid 1001)
FROM gotenberg-base AS production
COPY --from=apt-patch /usr /usr
COPY --from=apt-patch /var/lib/dpkg /var/lib/dpkg
COPY --from=go-patch --chmod=0755 /out/gotenberg-chromium /usr/bin/gotenberg
