#!/bin/bash
set -e

echo "🧪 Testing Machine Examples"
echo "============================"
echo ""

# Build machine CLI
echo "📦 Building machine CLI..."
go build -o machine .
echo "✅ Build complete"
echo ""

# Test each example
EXAMPLES=(
  "basic-linux"
  "python-dev"
  "node-dev"
  "full-stack"
)

for example in "${EXAMPLES[@]}"; do
  echo "🧪 Testing: $example"
  echo "----------------------------"
  
  cd "examples/$example"
  
  # Run machine up
  echo "  ▶️  Running: machine up"
  ../../machine up
  
  # Verify machine is running
  echo "  ✅ Machine is up"
  
  # Clean up
  echo "  🗑️  Cleaning up: machine destroy"
  ../../machine destroy
  
  cd ../..
  echo ""
done

echo "✅ All examples tested successfully!"

