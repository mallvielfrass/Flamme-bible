FROM golang:1.27.3-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/flammebible-server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 65532 app \
    && mkdir -p /data/media \
    && chown app:app /data/media

COPY --from=build /out/flammebible-server /usr/local/bin/flammebible-server

USER app
ENV ADDR=:8080 \
    MEDIA_DIR=/data/media
EXPOSE 8080
VOLUME ["/data/media"]
ENTRYPOINT ["flammebible-server"]
