FROM golang:1.13 AS build
ADD . /app/src
WORKDIR /app/src
RUN make

FROM ubuntu
LABEL description="csif-driver plugin"
COPY --from=build /app/src/bin/csif-plugin /csif-plugin
ENTRYPOINT ["/csif-plugin"]