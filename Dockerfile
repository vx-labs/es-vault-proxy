FROM vxlabs/dep as builder

WORKDIR $GOPATH/src/github.com/vx-labs/vault-config-extractor
COPY Gopkg* ./
RUN dep ensure -vendor-only
COPY . ./
RUN go test ./... && \
    go build -buildmode=exe -a -o /bin/es-vault-proxy ./main.go

FROM alpine
RUN apk -U add ca-certificates && \
    rm -rf /var/cache/apk/*
COPY --from=builder /bin/es-vault-proxy /bin/es-vault-proxy
ENTRYPOINT ["/bin/es-vault-proxy"]

