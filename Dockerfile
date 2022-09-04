FROM golang:1.19 as builder
WORKDIR /app
COPY . .
RUN go build -o ./bin ./cmd/...

FROM ubuntu:22.04
COPY --from=builder /app/bin/themis-server /usr/local/bin/themis-server
RUN apt update -y; apt install -y ca-certificates; apt-get clean
ENTRYPOINT ["themis-server", "-db=prod.db"]