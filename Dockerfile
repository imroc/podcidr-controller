FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o podcidr-controller .

FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /app/podcidr-controller .

USER 65532:65532

ENTRYPOINT ["/podcidr-controller"]
