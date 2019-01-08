FROM golang:alpine AS build

COPY . $GOPATH/src/chainspace.io/blockmania
RUN CGO_ENABLED=0 GOOS=linux go install -a -tags netgo -ldflags '-w' chainspace.io/blockmania/cmd/blockmania

FROM scratch

COPY --from=build /go/bin/blockmania /blockmania

ENTRYPOINT ["/blockmania"]
