# Define the path to the compiled Go binary
$binary = ".\fetch_github_token.exe"

# Check if the binary exists
if (-Not (Test-Path $binary)) {
    Write-Host "Error: Binary '$binary' not found. Please build it first." -ForegroundColor Red
    exit 1
}

# Run the binary and capture output
try {
    $output = & $binary 2>&1
    $exitCode = $LASTEXITCODE
} catch {
    Write-Host "Error: Failed to execute '$binary'." -ForegroundColor Red
    exit 1
}

# Check if execution was successful
if ($exitCode -ne 0) {
    Write-Host "Error: Failed to fetch GitHub App access token." -ForegroundColor Red
    Write-Host "Details: $output" -ForegroundColor Yellow
    exit $exitCode
}

# Extract the access token (assumes output contains "GitHub App Access Token: <token>")
$tokenLine = $output | Where-Object { $_ -match "GitHub App Access Token:" }
if ($tokenLine) {
    $token = $tokenLine -replace "GitHub App Access Token: ", ""
    Write-Host "Access Token: $token" -ForegroundColor Green
} else {
    Write-Host "Error: Failed to extract access token from response." -ForegroundColor Red
    exit 1
}