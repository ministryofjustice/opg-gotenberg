FROM gotenberg/gotenberg:8.39.0-chromium@sha256:558943677cf9092df83ff654c2645856dcb9ddf777f359243789688e74f3bcdd AS gotenberg-base

FROM gotenberg-base AS apt-patch
USER root
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends --only-upgrade \
        ca-certificates chromium chromium-common libexpat1; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

# potentially not worth patching
FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6 AS jar-patch
ARG COMMONS_LANG3_VERSION=3.18.0
ARG COMMONS_LANG3_SHA1=fb14946f0e39748a6571de0635acbe44e7885491
RUN apk add --no-cache curl zip unzip
COPY --from=gotenberg-base /usr/bin/pdftk-all.jar /work/pdftk-all.jar
WORKDIR /work
RUN set -eux; \
    curl -fsSLo cl3.jar https://repo1.maven.org/maven2/org/apache/commons/commons-lang3/${COMMONS_LANG3_VERSION}/commons-lang3-${COMMONS_LANG3_VERSION}.jar; \
    echo "${COMMONS_LANG3_SHA1}  cl3.jar" | sha1sum -c -; \
    mkdir new && unzip -q cl3.jar -d new 'org/*' 'META-INF/maven/org.apache.commons/*'; \
    zip -q -d pdftk-all.jar 'org/apache/commons/lang3/*' 'META-INF/maven/org.apache.commons/commons-lang3/*'; \
    (cd new && zip -q -r ../pdftk-all.jar .)

# build gotenberg binary from patched Go (ideally remove when gotenberg updates go)
FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4 AS go-patch
ARG GOTENBERG_VERSION=8.39.0
ARG PDFCPU_VERSION=v0.15.0
ENV CGO_ENABLED=0

ADD https://github.com/gotenberg/gotenberg.git#e4ee0f29d648f8d5a0a766dd797d108361905deb /src/gotenberg
ADD https://github.com/pdfcpu/pdfcpu.git#f2686555086a2e76dc19f602ea1897f6e3baae4d /src/pdfcpu

WORKDIR /src/gotenberg
RUN go get github.com/labstack/echo/v5@7c312e09d96ac13bdb6d32748dacbfcc689a2480 \
    && go mod tidy \
    && go mod download && go mod verify \
    && go build -o /out/gotenberg-chromium \
       -ldflags "-s -w -X 'github.com/gotenberg/gotenberg/v8/cmd.Version=${GOTENBERG_VERSION}'" \
       cmd/gotenberg-chromium/main.go

# build pdfcpu binary with from patched go
WORKDIR /src/pdfcpu
RUN go mod download && go mod verify \
    && go build -o /out/pdfcpu \
       -ldflags "-s -w -X 'main.version=${PDFCPU_VERSION}' -X 'github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model.VersionStr=${PDFCPU_VERSION}' -X main.builtBy=gotenberg" \
       ./cmd/pdfcpu \
    && /out/pdfcpu version

# base image already has non-root "gotenberg" user (uid/gid 1001)
FROM gotenberg-base AS production
COPY --from=apt-patch /usr /usr
COPY --from=apt-patch /var/lib/dpkg /var/lib/dpkg
COPY --from=jar-patch --chmod=0644 /work/pdftk-all.jar /usr/bin/pdftk-all.jar
COPY --from=go-patch --chmod=0755 /out/gotenberg-chromium /usr/bin/gotenberg
COPY --from=go-patch --chmod=0755 /out/pdfcpu /usr/bin/pdfcpu
