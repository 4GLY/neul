# neul collector (Windows PowerShell 5.1+). Sent over stdin: `ssh <host> powershell -NoProfile -NonInteractive -Command -`.
# Read-only. Same `@@ <section>` format as collect.sh; Windows reports the meta section only.
# Keep every statement on one line: `-Command -` executes stdin line by line.
$ErrorActionPreference = 'SilentlyContinue'
function kv($k, $v) { if ($null -ne $v -and "$v" -ne '') { Write-Output "$k=$v" } }
$os = Get-CimInstance Win32_OperatingSystem
$cs = Get-CimInstance Win32_ComputerSystem
$disk = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='$($env:SystemDrive)'"
$batt = Get-CimInstance Win32_Battery | Select-Object -First 1
Write-Output '@@ meta'
kv 'schema' 1
kv 'hostname' $env:COMPUTERNAME
kv 'os' 'Windows'
kv 'kernel' $os.Version
kv 'arch' $env:PROCESSOR_ARCHITECTURE
kv 'user' $env:USERNAME
kv 'collected_at' ((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))
kv 'os_name' $os.Caption
kv 'model' $cs.Model
kv 'cpu_count' $cs.NumberOfLogicalProcessors
if ($os -and $os.LastBootUpTime) { kv 'uptime_s' ([int64]((Get-Date) - $os.LastBootUpTime).TotalSeconds) }
if ($cs) { kv 'mem_total_b' ([int64]$cs.TotalPhysicalMemory) }
if ($os) { kv 'mem_available_b' ([int64]$os.FreePhysicalMemory * 1024) }
if ($disk) { kv 'disk_path' $disk.DeviceID; kv 'disk_total_b' ([int64]$disk.Size); kv 'disk_used_b' ([int64]$disk.Size - [int64]$disk.FreeSpace); kv 'disk_available_b' ([int64]$disk.FreeSpace) }
if ($batt) { $state = if ($batt.BatteryStatus -eq 2) { 'AC attached' } else { 'discharging' }; kv 'battery' "$($batt.EstimatedChargeRemaining) $state" }
Write-Output '@@ end'
