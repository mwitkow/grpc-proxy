#!/bin/bash
# Regenerate the self-signed certificate for local host.

#set -e
set -x

openssl genrsa -out ca.key 4096   # note, foo is the password to ca.key
openssl req -nodes -new -x509 -days 1024 -key ca.key -out ca.crt \
 -subj "/C=UK/O=CA/CN=test.com Self-Signed CA"

echo "Generating Client Cert  cert (ca.key and ca.crt)"
openssl genrsa -out client.key  4096
openssl req -nodes  -new -key client.key -out client.csr  \
 -subj "/C=UK/O=admins/O=eng/CN=someone@test.com/EA=someone@test.com"
openssl x509 -req -days 1024 -in client.csr -CA ca.crt -CAkey ca.key -passin pass:foo -set_serial 01 -out client.crt

echo "Generating a self signed server cert, without CA"
openssl req -x509 -sha256 -nodes -newkey rsa:2048 -days 1024 -keyout localhost.key -out localhost.crt \
 -subj "/C=UK/CN=localhost"

#echo "Generating CA cert (ca.key and ca.crt)"
#openssl genrsa -des3 -passout pass:foo -out ca.key 4096   # note, foo is the password to ca.key
#openssl req -new -x509 -days 1024 -key ca.key -passin pass:foo -out ca.crt \
# -subj "/C=UK/O=CA/CN=test.com Self-Signed CA"
#
#echo "Generating Client Cert  cert (ca.key and ca.crt)"
#openssl genrsa -des3 -out client.key -passout pass:bar  4096
#openssl req -new -key client.key -out client.csr -passin pass:bar \
# -subj "/C=UK/O=admins/O=eng/CN=someone@test.com/EA=someone@test.com"
#openssl x509 -req -days 1024 -in client.csr -CA ca.crt -CAkey ca.key -passin pass:foo -set_serial 01 -out client.crt
#
#echo "Generating a self signed server cert, without CA"
#openssl req -x509 -sha256 -nodes -newkey rsa:2048 -days 1024 -keyout localhost.key -out localhost.crt \
# -subj "/C=UK/CN=localhost"
#
