#!/usr/bin/env bash

OUTPUT_DIR=$1

# generate CA
openssl genrsa -out ${OUTPUT_DIR}/rootCA.key 2048

openssl req -x509 -new -nodes -key ${OUTPUT_DIR}/rootCA.key -subj "/C=CN/ST=BJ/O=tencent, Inc./CN=lbcf-controller.kube-system.svc" -sha256 -days 1024 -out ${OUTPUT_DIR}/rootCA.crt

# generate private key for server
openssl genrsa -out ${OUTPUT_DIR}/server.key 2048

# generate csr
openssl req -new -sha256 -key ${OUTPUT_DIR}/server.key -subj "/C=CN/ST=BJ/O=tencent, Inc./CN=lbcf-controller.kube-system.svc" -out ${OUTPUT_DIR}/server.csr

# generate certificate for server
openssl x509 -req -in ${OUTPUT_DIR}/server.csr -CA ${OUTPUT_DIR}/rootCA.crt -CAkey ${OUTPUT_DIR}/rootCA.key -CAcreateserial -out ${OUTPUT_DIR}/server.crt -days 500 -sha256
