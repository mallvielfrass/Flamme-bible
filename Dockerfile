FROM golang:1.27.3-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/flammebible-server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/flammebible-reindex ./cmd/reindex

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 65532 app \
    && mkdir -p /data/media /data/releases \
    && chown app:app /data/media /data/releases

COPY --from=build /out/flammebible-server /usr/local/bin/flammebible-server
COPY --from=build /out/flammebible-reindex /usr/local/bin/flammebible-reindex

USER app
ENV ADDR=:8080 \
    MEDIA_DIR=/data/media \
    RELEASES_DIR=/data/releases
EXPOSE 8080
VOLUME ["/data/media", "/data/releases"]
ENTRYPOINT ["flammebible-server"]
