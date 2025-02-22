#!/bin/bash

# Define variables
APP_ID="1128251"
INSTALLATION_ID="60279689"
PRIVATE_KEY_PATH="/Users/akashshrivastava/Downloads/cgi-test-akash-harness.2025-01-30.private-key.pem"

# Path to the compiled Go binary
BINARY="./fetch_github_token"

# Check if the binary exists
if [[ ! -x "$BINARY" ]]; then
    echo "Error: Binary '$BINARY' not found or not executable. Please build it first."
    exit 1
fi

# Run the binary with arguments
OUTPUT=$($BINARY -appId="$APP_ID" -appInstallationId="$INSTALLATION_ID" -privateKey="$PRIVATE_KEY_PATH" 2>&1)
EXIT_CODE=$?

# Check if execution was successful
if [[ $EXIT_CODE -ne 0 ]]; then
    echo "Error: Failed to fetch GitHub App access token."
    echo "Details: $OUTPUT"
    exit $EXIT_CODE
fi

# Extract the access token
TOKEN=$(echo "$OUTPUT" | grep "GitHub App Access Token:" | awk '{print $5}')

# Validate token extraction
if [[ -z "$TOKEN" ]]; then
    echo "Error: Failed to extract access token from response."
    exit 1
fi

# Print the token
echo "$TOKEN"