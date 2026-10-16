FROM golang:1.27.2-trixie

RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils \
    && rm -rf /var/lib/apt/lists/*

RUN go install gotest.tools/gotestsum@v1.13.0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

ENTRYPOINT ["gotestsum", "--format", "testname", "--junitfile", "test-results/junit/opg-gotenberg-visual.xml", "--", "-count=1", "./..."]
