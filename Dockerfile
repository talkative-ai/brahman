FROM golang:alpine

RUN apk add --update git

COPY docker.gitconfig /root/.gitconfig

RUN go get github.com/artificial-universe-maker/brahman

ENTRYPOINT /go/bin/brahman

EXPOSE 8080