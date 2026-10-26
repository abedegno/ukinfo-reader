# Build with the server directory as context. No media or private workloads.
FROM golang:1.26.10-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ukinfo-search-engine-spike ./cmd/ukinfo-search-engine-spike
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /client ./cmd/ukinfo-search-engine-spike
FROM scratch
COPY --from=build /client /client
USER 1000:10
WORKDIR /work
ENTRYPOINT ["/client"]
