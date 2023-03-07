FROM golang:1.19-buster as builder
WORKDIR /app
COPY . .
RUN go build -buildvcs=false -o ./bin ./cmd/...

FROM ubuntu:22.04 as litestream
WORKDIR /download
RUN apt update -y && apt install -y wget tar
RUN wget https://github.com/benbjohnson/litestream/releases/download/v0.3.9/litestream-v0.3.9-linux-amd64.tar.gz; \
  tar -zxf litestream-v0.3.9-linux-amd64.tar.gz;

FROM ubuntu:22.04
WORKDIR /themis
COPY --from=builder /app/bin/themis-server /usr/local/bin/themis-server
COPY --from=litestream /download/litestream /usr/local/bin/litestream
COPY --from=builder /app/start.sh ./start.sh
# install ca-certificates for outbound https calls, and sqlite3 for debugging
RUN apt update -y; apt install -y ca-certificates sqlite3; apt-get clean
ENTRYPOINT ["./start.sh"]
