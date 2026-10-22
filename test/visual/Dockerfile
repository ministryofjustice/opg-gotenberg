FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4

RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils \
    && rm -rf /var/lib/apt/lists/*

RUN go install gotest.tools/gotestsum@v1.13.0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

ENTRYPOINT ["gotestsum", "--format", "testname", "--junitfile", "test-results/junit/opg-gotenberg-visual.xml", "--", "-count=1", "./..."]
