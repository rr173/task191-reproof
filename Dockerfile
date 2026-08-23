FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/task191-reproof ./cmd/reproof

ENTRYPOINT ["/app/task191-reproof"]
CMD ["--smoke-test"]
