FROM golang:1.20
WORKDIR /srv

COPY cmd cmd/
COPY internal internal/
COPY pkg pkg/

COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go build -o server cmd/main/main.go

CMD ["./server"]
