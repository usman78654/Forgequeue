$ErrorActionPreference = 'Stop'
docker compose up -d --build
$api = 'http://localhost:18080'
$headers = @{Authorization='Bearer fq_admin_dev'; 'Content-Type'='application/json'}
$deadline = (Get-Date).AddMinutes(3)
do { try { $ready = Invoke-RestMethod "$api/readyz"; break } catch { Start-Sleep 2 } } while ((Get-Date) -lt $deadline)
if (-not $ready) { throw 'ForgeQueue did not become ready' }
$job = Invoke-RestMethod -Method Post -Uri "$api/v1/jobs" -Headers $headers -Body '{"type":"echo","payload":{"message":"portfolio demo"},"priority":9,"idempotency_key":"portfolio-demo"}'
$workflowBody = Get-Content examples/workflow.json -Raw
$workflow = Invoke-RestMethod -Method Post -Uri "$api/v1/workflows" -Headers $headers -Body $workflowBody
Write-Host "Job:      $api/v1/jobs/$($job.id)"
Write-Host "Workflow: $api/v1/workflows/$($workflow.id)"
Write-Host "Console:  $api/app/"
Write-Host 'Grafana:  http://localhost:3000  (admin/admin)'
Write-Host 'MinIO:    http://localhost:9001  (minioadmin/minioadmin)'
Write-Host 'RabbitMQ: http://localhost:15672 (guest/guest)'
