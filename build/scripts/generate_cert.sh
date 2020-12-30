#!/usr/bin/env bash
cd $(dirname "$0")

OUTPUT_DIR=$1
SANCNF=san.cnf

mkdir -p ${OUTPUT_DIR}

cat << EOF > ${SANCNF}
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
C = CN
O = Tencent
CN = LBCF

[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1=lbcf-controller.kube-system.svc
EOF

set -e
# generate CA
openssl genrsa -out ${OUTPUT_DIR}/rootCA.key 2048

openssl req -x509 -new -nodes -key ${OUTPUT_DIR}/rootCA.key -config ${SANCNF} -sha256 -days 36500 -out ${OUTPUT_DIR}/rootCA.crt

# generate private key for server
openssl genrsa -out ${OUTPUT_DIR}/server.key 2048

# generate csr
openssl req -new -key ${OUTPUT_DIR}/server.key -out ${OUTPUT_DIR}/server.csr -config ${SANCNF} -extensions v3_req

# generate certificate for server
openssl x509 -req -in ${OUTPUT_DIR}/server.csr -CA ${OUTPUT_DIR}/rootCA.crt -CAkey ${OUTPUT_DIR}/rootCA.key -CAcreateserial -out ${OUTPUT_DIR}/server.crt -days 36500 -sha256 -extfile ${SANCNF} -extensions v3_req

rm san.cnf