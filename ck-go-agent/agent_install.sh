make docker-build
docker exec -it kind-control-plane crictl rmi otel-go-instrumentation
kind load docker-image otel-go-instrumentation
