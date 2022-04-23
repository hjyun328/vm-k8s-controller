#!/bin/bash
# CREATE PRIVATE KEY FOR CA
openssl genrsa -out ca.key 2048

# CREATE CERTIFICATE FOR CA
openssl req -new -x509 -key ca.key -out ca.crt -config ca.conf

# CREATE PRIVATE KEY FOR WEBHOOK
openssl genrsa -out webhook.key 2048

# CREATE CERTIFICATE SIGN REQUEST FOR WEBHOOK
openssl req -new -key webhook.key -out webhook.csr -config webhook.conf

# CREATE CERTIFICATE FOR WEBHOOK WITH CA
openssl x509 -req -in webhook.csr -CA ca.crt -CAkey ca.key -CAcreateserial -extensions v3_req -extfile webhook.conf -out webhook.crt
