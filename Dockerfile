FROM golang:1.24.3-alpine3.21 as builder

# We assume only git is needed for all dependencies.
# openssl is already built-in.
RUN apk add -U --no-cache git

WORKDIR /opt/wiilink/prod/Mail-Server/

# Cache pulled dependencies if not updated.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy necessary parts of the Mail-Go source into builder's source
COPY *.go ./

# Build to name "app".
RUN go build -o app .

EXPOSE 6666
# Wait until there's an actual MySQL connection we can use to start.
CMD ["./app"]
