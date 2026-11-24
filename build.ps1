# Build and load the multi-platform image (requires a containerd image store).
docker buildx build --load -t otelcol-opcua:0.0.1 --platform=linux/amd64,linux/arm/v7,linux/arm64,linux/ppc64le,linux/s390x .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Test the newly built image
docker run -it --rm -p 4317:4317 -p 4318:4318 --name otelcol otelcol-opcua:0.0.1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }