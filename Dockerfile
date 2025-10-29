ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /run-app ./cmd/bd


FROM debian:bookworm

COPY --from=builder /run-app /usr/local/bin/run-app
RUN chmod +x /usr/local/bin/run-app

# Create the data directory
RUN mkdir -p /data

# Set environment variables
ENV PORT=8080
ENV BEADS_API_SECRET=""
ENV BEADS_DB="/data/beads.db"

# Expose the port
EXPOSE 8080

# Set working directory
WORKDIR /data

# Run the server
CMD ["run-app", "serve", "--host", "0.0.0.0", "--port", "8080"]
