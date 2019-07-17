# External Scaler for KEDA Example

This is an example of using an external scaler with KEDA. This scaler implements the gRPC interface. 

## Getting Started

1. Install KEDA

2. Create self signed certificates for TLS

```sh
# Generate Private key and CSR
# Set the common name when prompted to redis-external-scaler-service
# which is the name of the service in kubernetes
openssl req -new > server.csr
openssl rsa -in privkey.pem -out server.key

# Generate Public key
openssl x509 -in server.csr -out server.crt -req -signkey server.key -days 365
```

3. Generate a secret with the certs
```sh
kubectl create secret generic keda-redis-external-scaler \
    --namespace keda \
    --from-file=server.key \
    --from-file=server.crt
```

4. Deploy the gRPC server for the external scaler. This creates a service and a deployment
```sh
kubectl apply -f manifests/
```