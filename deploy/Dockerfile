FROM golang:1.26.9-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /ukinfo-server ./cmd/ukinfo-server

RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /ukinfo-parade-index ./cmd/ukinfo-parade-index

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 ukinfo
COPY --from=build /ukinfo-server /usr/local/bin/ukinfo-server
COPY --from=build /ukinfo-parade-index /usr/local/bin/ukinfo-parade-index
USER ukinfo
EXPOSE 8080
ENTRYPOINT ["ukinfo-server"]
CMD ["serve", "--listen", "0.0.0.0:8080", "--public-origin", "http://localhost:8080"]
