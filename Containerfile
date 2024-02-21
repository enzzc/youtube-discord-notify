FROM alpine:3.19
ADD bin/ydn ydn
ENTRYPOINT ["./ydn"]
