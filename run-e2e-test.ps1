# run-e2e-test.ps1
# Runs the E2E test locally on Windows with Docker Desktop.
# Usage: .\run-e2e-test.ps1

param(
    [int]$WaitSeconds = 30
)

$ErrorActionPreference = "Stop"
$ProjectRoot = $PSScriptRoot

Write-Host "=== OPC UA E2E Test ===" -ForegroundColor Cyan

# --- Cleanup function ---
function Cleanup {
    Write-Host "`nCleaning up..." -ForegroundColor Yellow
    docker rm -f testserver otelcol 2>$null | Out-Null
    docker network rm opcua-test-net 2>$null | Out-Null
    docker volume rm otel-output 2>$null | Out-Null
}

# Clean up any leftovers from a previous run
Cleanup

try {
    # 1. Build images
    Write-Host "`n[1/8] Building test server image..." -ForegroundColor Green
    docker build -t opcua-testserver "$ProjectRoot/testserver"
    if ($LASTEXITCODE -ne 0) { throw "Failed to build test server image" }

    Write-Host "`n[2/8] Building collector image..." -ForegroundColor Green
    docker build -t otelcol-opcua "$ProjectRoot"
    if ($LASTEXITCODE -ne 0) { throw "Failed to build collector image" }

    # 2. Create network and volume
    Write-Host "`n[3/8] Creating Docker network and volume..." -ForegroundColor Green
    docker network create opcua-test-net | Out-Null
    docker volume create otel-output | Out-Null

    # 3. Start test server
    Write-Host "`n[4/8] Starting OPC UA test server..." -ForegroundColor Green
    docker run -d --name testserver --network opcua-test-net opcua-testserver | Out-Null
    Write-Host "Waiting 15s for server startup..."
    Start-Sleep -Seconds 15

    Write-Host "`nTest server logs:" -ForegroundColor Gray
    docker logs testserver

    # 4. Start collector
    Write-Host "`n[5/8] Starting OTel Collector..." -ForegroundColor Green
    $configPath = "$ProjectRoot/testserver/ci-collector-config.yaml" -replace '\\','/'
    docker run -d --name otelcol `
        --network opcua-test-net `
        --user 0 `
        -v "${configPath}:/otelcol/collector-config.yaml:ro" `
        -v "otel-output:/output" `
        otelcol-opcua `
        --config /otelcol/collector-config.yaml | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Failed to start collector" }

    # 5. Wait for collection
    Write-Host "`n[6/8] Waiting ${WaitSeconds}s for collection cycle..." -ForegroundColor Green
    Start-Sleep -Seconds $WaitSeconds

    # 6. Show collector logs
    Write-Host "`nCollector logs:" -ForegroundColor Gray
    docker logs otelcol

    # 7. Stop collector to flush file exporter
    Write-Host "`n[7/8] Stopping collector (flush file exporter)..." -ForegroundColor Green
    docker stop otelcol | Out-Null
    Start-Sleep -Seconds 5

    # 8. Validate output
    Write-Host "`n[8/8] Validating output..." -ForegroundColor Green
    $validatePath = "$ProjectRoot/testserver" -replace '\\','/'
    docker run --rm `
        -v "otel-output:/output" `
        -v "${validatePath}:/testserver" `
        alpine sh -c "apk add --no-cache jq bash && bash /testserver/validate.sh /output/logs.json /testserver/expected/records.json"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "`nE2E TEST FAILED" -ForegroundColor Red

        # Copy output for inspection
        $outputPath = "$ProjectRoot/e2e-output.json" -replace '\\','/'
        docker run --rm -v "otel-output:/output" -v "${ProjectRoot}:/host" `
            alpine cp /output/logs.json /host/e2e-output.json 2>$null
        if (Test-Path "$ProjectRoot/e2e-output.json") {
            Write-Host "Collector output saved to: e2e-output.json" -ForegroundColor Yellow
        }
        exit 1
    }

    Write-Host "`nE2E TEST PASSED" -ForegroundColor Green
}
catch {
    Write-Host "`nERROR: $_" -ForegroundColor Red

    Write-Host "`n--- Test server logs ---" -ForegroundColor Yellow
    docker logs testserver 2>$null
    Write-Host "`n--- Collector logs ---" -ForegroundColor Yellow
    docker logs otelcol 2>$null

    exit 1
}
finally {
    Cleanup
}
