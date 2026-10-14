param(
    [string]$ApiUrl = 'http://localhost:18080',
    [string]$ApiKey = $env:FORGEQUEUE_API_KEY,
    [switch]$StartWorker,
    [switch]$InvalidData
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
if (-not $ApiKey) { throw 'Set FORGEQUEUE_API_KEY to your local development API key first.' }
$ApiUrl = $ApiUrl.TrimEnd('/')
$headers = @{Authorization="Bearer $ApiKey"; 'Content-Type'='application/json'}
Invoke-RestMethod "$ApiUrl/readyz" | Out-Null
if ($StartWorker) {
    docker compose -f (Join-Path $repoRoot 'compose.reports.yaml') up -d report-worker
    if ($LASTEXITCODE -ne 0) { throw 'Report worker could not start.' }
}
$filename = if ($InvalidData) { 'report-invalid-workflow.json' } else { 'report-workflow.json' }
$body = Get-Content (Join-Path $repoRoot "examples/$filename") -Raw -Encoding UTF8
$workflow = Invoke-RestMethod -Method Post -Uri "$ApiUrl/v1/workflows" -Headers $headers -Body $body
Write-Host "Submitted $($workflow.name): $($workflow.id)"
Write-Host "Watch it in $ApiUrl/app/#/workflows"
$deadline = (Get-Date).AddMinutes(2)
do {
    $current = Invoke-RestMethod "$ApiUrl/v1/workflows/$($workflow.id)" -Headers $headers
    if ($current.status -in @('succeeded','failed','cancelled')) { break }
    Start-Sleep -Milliseconds 500
} while ((Get-Date) -lt $deadline)
$current.steps | Select-Object key, status, job_id | Format-Table
$expected = if ($InvalidData) { 'failed' } else { 'succeeded' }
if ($current.status -ne $expected) { throw "Expected $expected, got $($current.status). Check that the report worker is running." }
if ($InvalidData) {
    $aggregate = $current.steps | Where-Object key -eq 'aggregate'
    $job = Invoke-RestMethod "$ApiUrl/v1/jobs/$($aggregate.job_id)" -Headers $headers
    if ($job.attempt -ne 1 -or $job.status -ne 'dead') { throw 'Permanent validation failure did not stop after one attempt.' }
    Write-Host "Expected validation failure: $($job.last_error)"
} else {
    $step = $current.steps | Where-Object key -eq 'report'
    $job = Invoke-RestMethod "$ApiUrl/v1/jobs/$($step.job_id)" -Headers $headers
    if ($job.result.total_cents -ne 48500 -or $job.result.order_count -ne 5) { throw 'Unexpected report totals.' }
    Write-Host $job.result.markdown
    Write-Host "Report generated in demo-output/reports (Docker worker), or REPORT_OUTPUT_DIR (native worker)."
}
