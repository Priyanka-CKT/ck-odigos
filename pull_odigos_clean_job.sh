#!/bin/sh

# Global variables
ODIGOS_VERSION="v1.0.142"
CK_VERSION="v1.0.144"

echo "Pulling Docker image keyval/odigos-cli:${ODIGOS_VERSION}..."
docker pull keyval/odigos-cli:${ODIGOS_VERSION}

echo "Inspecting Docker image manifests..."
docker manifest inspect docker.io/keyval/odigos-cli:${ODIGOS_VERSION}

echo "Tagging Docker image for ghcr.io repository..."
docker tag keyval/odigos-cli:${ODIGOS_VERSION} ghcr.io/sabareesh-ckt/codekarma/karmaclean:${CK_VERSION}

echo "Setting up Docker buildx for multi-architecture support..."
# Check if builder already exists
if ! docker buildx inspect mybuilder > /dev/null 2>&1; then
  echo "Creating new buildx builder 'mybuilder'..."
  docker buildx create --name mybuilder --use
else
  echo "Builder 'mybuilder' already exists, using it..."
  docker buildx use mybuilder
fi
docker buildx inspect --bootstrap

echo "Creating and pushing multi-architecture image to ghcr.io..."
docker buildx imagetools create --tag ghcr.io/sabareesh-ckt/codekarma/karmaclean:${CK_VERSION} keyval/odigos-cli:${ODIGOS_VERSION}

echo "Docker image processing completed successfully."

# Push karmaset
echo "Starting make push-karmaset..."
make push-karmaset
echo "Completed make push-karmaset."

# Push karmatap
echo "Starting make push-karmatap..."
make push-karmatap
echo "Completed make push-karmatap."

# Push karmadash
echo "Starting make push-karmadash..."
make push-karmadash
echo "Completed make push-karmadash."

echo "All push operations completed successfully."

