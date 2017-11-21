FROM golang:1.8-alpine
ADD . /go/src/web-echo
RUN go install web-echo

FROM alpine:latest
COPY --from=0 /go/bin/web-echo .
ENV PORT 8080
CMD ["./web-echo"]
