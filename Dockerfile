FROM golang:1.9.2 as builder

COPY ./ /go/src/github.com/jetstack/vault-unsealer/
WORKDIR /go/src/github.com/jetstack/vault-unsealer

RUN make go_verify
RUN make go_build

FROM alpine:3.6
RUN apk add --update ca-certificates
COPY --from=builder /go/src/github.com/jetstack/vault-unsealer/vault-unsealer_linux_amd64 /usr/local/bin/vault-unsealer
