FROM golang:1.26-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /ukinfo-server ./cmd/ukinfo-server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 ukinfo
COPY --from=build /ukinfo-server /usr/local/bin/ukinfo-server
USER ukinfo
EXPOSE 8080
ENTRYPOINT ["ukinfo-server"]
CMD ["serve", "--listen", "0.0.0.0:8080", "--public-origin", "http://localhost:8080"]
