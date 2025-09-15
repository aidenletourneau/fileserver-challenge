
FROM golang:1.25.1


# for debugging
RUN go install github.com/go-delve/delve/cmd/dlv@latest



WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -gcflags="all=-N -l" -o /go-middleware .


EXPOSE 8080
EXPOSE 40000


CMD ["dlv", "debug", "/go-middleware", "--headless", "--listen=:40000", "--api-version=2", "--accept-multiclient", "--log"]