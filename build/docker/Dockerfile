FROM alpine:3.9.4

COPY lbcf-controller /usr/local/bin/

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

CMD ["/usr/local/bin/lbcf-controller"]
