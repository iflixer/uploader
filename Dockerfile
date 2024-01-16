# build environment
FROM golang:1.18 as build-env
WORKDIR /server
COPY src/go.mod ./
RUN go mod download
COPY src src
WORKDIR /server/src
RUN CGO_ENABLED=0 GOOS=linux go build -o /server/build/httpserver .

FROM linuxserver/ffmpeg
WORKDIR /app

COPY --from=build-env /server/build/httpserver /app/httpserver
COPY --from=build-env /server/src/test.txt /app/test.txt

#ENV GITHUB-SHA=<GITHUB-SHA>

ENTRYPOINT [ "/app/httpserver" ]
#ENTRYPOINT [ "ls", "-la", "/app/httpserver" ]
