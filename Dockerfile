# syntax=docker/dockerfile:1.6

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/agent-sidecar ./cmd/aggregator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/agent-sidecar /agent-sidecar
USER nonroot:nonroot
ENTRYPOINT ["/agent-sidecar"]
