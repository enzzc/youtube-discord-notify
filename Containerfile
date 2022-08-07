FROM alpine:3.15
ADD bin/ydn ydn
ENTRYPOINT ["./ydn"]
