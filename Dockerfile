FROM golang:1.13-alpine AS build

ENV GOPATH "/go"
ENV PROJECT_PATH "$GOPATH/src/github.com/xenolog/ipam"
ENV GO111MODULE "on"

COPY . $PROJECT_PATH
RUN cd $PROJECT_PATH && make BINNAME=inwinstack-ipam \
  && mv out/inwinstack-ipam /tmp/inwinstack-ipam

# Running stage
FROM alpine:3.9
COPY --from=build /tmp/inwinstack-ipam /bin/inwinstack-ipam
ENTRYPOINT ["inwinstack-ipam"]