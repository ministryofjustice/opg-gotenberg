FROM gotenberg/gotenberg:8.40.0-chromium@sha256:a03fb9f9a7db6e1ff9b798137f3d9f60601ded73fe626b10fa72c920360b0efd AS gotenberg-base

FROM gotenberg-base AS apt-patch
USER root
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends --only-upgrade \
        ca-certificates chromium chromium-common curl expat libexpat1 libxml2 zlib1g; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

# potentially not worth patching
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS jar-patch
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
FROM golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS go-patch
ARG BROTOLI_COMMIT=33c2a9884269798192b5ecf171288f98939dbe42
ARG ECHO_COMMIT=a6a9673a54d2c719ceb32ce6a929d35483acfb2b
ARG GO_JOSE_COMMIT=a0111f82742fa92bceec9ede40845e73319e6230
ARG GOTENBERG_VERSION=8.39.0
ARG KLAUSPOST_COMPRESS_COMMIT=7f5d8528720c895a6106f201719f87eb030a565d
ARG PDFCPU_VERSION=v0.15.0
ARG X_NET_COMMIT=3de643db506517b3e652a13cc0b56034cea6b8e6
ENV CGO_ENABLED=0

ADD https://github.com/gotenberg/gotenberg.git#e4ee0f29d648f8d5a0a766dd797d108361905deb /src/gotenberg
ADD https://github.com/pdfcpu/pdfcpu.git#f2686555086a2e76dc19f602ea1897f6e3baae4d /src/pdfcpu

WORKDIR /src/gotenberg
RUN go get github.com/labstack/echo/v5@${ECHO_COMMIT} \
        golang.org/x/net@${X_NET_COMMIT} \
        github.com/klauspost/compress@${KLAUSPOST_COMPRESS_COMMIT} \
        github.com/go-jose/go-jose/v4@${GO_JOSE_COMMIT} \
        github.com/andybalholm/brotli@${BROTOLI_COMMIT} \
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
