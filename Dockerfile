FROM golang:latest
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o exporter main.go
EXPOSE 3000
ENTRYPOINT [ "/app/exporter" ]
