FROM golang:alpine

RUN apk add --update git

COPY docker.gitconfig /root/.gitconfig

RUN go get github.com/artificial-universe-maker/brahman

RUN cp /go/src/github.com/artificial-universe-maker/brahman/auth.html /go/bin/

ENTRYPOINT /go/bin/brahman

EXPOSE 8080