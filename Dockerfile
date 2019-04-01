FROM golang:1.10-alpine as builder
COPY . /go/src/github.com/tibold/docker-volume-shared
WORKDIR /go/src/github.com/tibold/docker-volume-shared
RUN set -ex \
    && apk add --no-cache --virtual .build-deps \
    gcc libc-dev \
    && go install --ldflags '-extldflags "-static"' \
    && apk del .build-deps
CMD ["/go/bin/docker-volume-shared"]

FROM alpine
RUN mkdir -p /run/docker/plugins /volumes
COPY --from=builder /go/bin/docker-volume-shared .
CMD ["docker-volume-shared"]