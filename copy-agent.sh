#!/bin/bash

# Create ck-go-agent directory in the root directory
mkdir -p ck-go-agent
cp -r ../ck-go-agent/* ck-go-agent/
rm -rf ck-go-agent/.git

# Ensure the go.mod file exists and has the correct module name
if [ -f ck-go-agent/go.mod ]; then
    # Update the module name in go.mod if needed
    sed -i '' 's|^module .*|module go.opentelemetry.io/auto|' ck-go-agent/go.mod
fi 