param([ValidateSet('worker','broker','database')] [string]$Target = 'worker')
$ErrorActionPreference = 'Stop'
$api = if ($env:FORGEQUEUE_URL) { $env:FORGEQUEUE_URL } else { 'http://localhost:18080' }
$headers = @{'Content-Type'='application/json'}
if ($env:FORGEQUEUE_API_KEY) { $headers.Authorization = "Bearer $env:FORGEQUEUE_API_KEY" }
$job = Invoke-RestMethod -Method Post -Uri "$api/v1/jobs" -Headers $headers -Body '{"type":"sleep","payload":{"duration":"12s"},"timeout_seconds":60}'
Start-Sleep -Seconds 2
$service = @{worker='worker'; broker='rabbitmq'; database='postgres'}[$Target]
docker compose stop $service
Start-Sleep -Seconds 5
docker compose start $service
$deadline = (Get-Date).AddMinutes(2)
do { Start-Sleep -Seconds 2; $state = Invoke-RestMethod -Uri "$api/v1/jobs/$($job.id)" -Headers $headers } while ($state.status -notin @('succeeded','failed','dead','cancelled') -and (Get-Date) -lt $deadline)
$state | ConvertTo-Json -Depth 5
if ($state.status -ne 'succeeded') { throw "Recovery failed: $($state.status)" }
