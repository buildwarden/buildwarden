<#
.SYNOPSIS
Installs, checks or removes the scheduled native session on a Windows host (FLOW-09).

.DESCRIPTION
The session itself is native-tick.sh, run under Git Bash (which Claude Code on
Windows already needs). This script checks the host and registers a Task
Scheduler task that runs it. Run it from the dedicated checkout the session
will use, never from a working copy you edit: every run resets that checkout to
origin/main.

  .\tools\workflow\native\install-windows.ps1 check
  .\tools\workflow\native\install-windows.ps1 install [-Machine windows-desktop] [-At 04:10] [-EveryHours 6]
  .\tools\workflow\native\install-windows.ps1 uninstall

The task runs as you, only while you are logged on (no stored password), and
does not run elevated: Hyper-V work needs your account in the local
"Hyper-V Administrators" group instead of admin rights.
#>
param(
    [Parameter(Position = 0)][ValidateSet('check', 'install', 'uninstall')][string]$Command = 'check',
    [string]$Machine = 'windows-desktop',
    [string]$At = '04:10',
    [int]$EveryHours = 6
)
$ErrorActionPreference = 'Stop'
$TaskName = 'BuildWarden native tick'
$Repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$script:Problems = 0
function Ok($m) { Write-Host "  ok    $m" }
function Bad($m) { Write-Host "  FAIL  $m"; $script:Problems++ }
function Warn($m) { Write-Host "  warn  $m" }

function Find-Bash {
    foreach ($p in @("$env:ProgramFiles\Git\bin\bash.exe", "${env:ProgramFiles(x86)}\Git\bin\bash.exe",
            "$env:LOCALAPPDATA\Programs\Git\bin\bash.exe")) {
        if ($p -and (Test-Path $p)) { return $p }
    }
    return $null
}

# Runs a command in Git Bash with the PATH native-tick.sh uses; returns its output, $null on failure.
function In-Bash($bash, $cmd) {
    $out = & $bash -lc "export PATH=`"`$HOME/.local/bin:`$PATH`"; $cmd" 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    return ($out -join "`n")
}

function Check {
    Write-Host "Checking $Repo for machine $Machine"
    $bash = Find-Bash
    if (-not $bash) { Bad 'Git Bash not found (install Git for Windows; Claude Code needs it too)'; return }
    Ok "Git Bash: $bash"
    $script:Bash = $bash
    $script:UnixRepo = In-Bash $bash "cygpath -u '$Repo'"
    foreach ($t in 'git', 'go', 'make', 'claude') {
        $p = In-Bash $bash "command -v $t"
        if ($p) { Ok "${t}: $p" } else { Bad "$t not on PATH in Git Bash" }
    }
    if (In-Bash $bash "cd '$($script:UnixRepo)/tools/workflow' && go version") { Ok 'go builds tools/workflow' }
    else { Bad 'go cannot build tools/workflow (version in tools/workflow/go.mod)' }
    if (In-Bash $bash 'command -v entire') { Ok 'entire: sessions will be recorded (FLOW-18)' }
    else { Warn 'entire not installed: sessions will not be recorded (FLOW-18)' }

    # windows-hyperv
    if (Get-Command Get-VMHost -ErrorAction SilentlyContinue) {
        try { $null = Get-VMHost; Ok 'Hyper-V reachable without elevation (windows-hyperv)' }
        catch { Bad 'Get-VMHost failed: add yourself to "Hyper-V Administrators" and log off/on' }
    } else { Bad 'Hyper-V PowerShell module missing (windows-hyperv)' }
    # qemu-any
    if (In-Bash $bash 'command -v qemu-system-x86_64') { Ok 'qemu (qemu-any); WHPX acceleration needs the Windows Hypervisor Platform feature' }
    else { Warn 'no qemu-system-x86_64 in Git Bash PATH: qemu-any work will fail (or move qemu-any to another machine in hosts.yaml)' }
    # container-any
    $rt = $null
    foreach ($r in 'docker', 'podman', 'finch') {
        if (In-Bash $bash "command -v $r && $r info") { $rt = $r; break }
    }
    if ($rt) { Ok "container runtime: $rt (container-any)" }
    else { Warn 'no working docker/podman/finch: container-any work will fail (or move container-any to the macbook in hosts.yaml)' }

    $url = In-Bash $bash "git -C '$($script:UnixRepo)' remote get-url origin"
    if ($url -match 'buildwarden/buildwarden(\.git)?$') { Ok "origin is $url" } else { Bad "origin is '$url', want buildwarden/buildwarden" }
    if ((In-Bash $bash "git -C '$($script:UnixRepo)' status --porcelain --untracked-files=no") -eq '') { Ok 'checkout is clean' }
    else { Bad 'checkout has local changes; use a dedicated clone' }
    if ($null -ne (In-Bash $bash "git -C '$($script:UnixRepo)' fetch -q origin workflow")) { Ok 'can fetch origin' } else { Bad 'cannot fetch origin' }
    if ($null -ne (In-Bash $bash "git -C '$($script:UnixRepo)' push --dry-run -q origin refs/remotes/origin/workflow:refs/heads/workflow")) {
        Ok 'can push the workflow branch'
    } else { Bad 'cannot push the workflow branch (credentials?)' }
    $hosts = In-Bash $bash "git -C '$($script:UnixRepo)' show origin/workflow:hosts.yaml"
    if ($hosts -match "name: $Machine(\r?\n|$)") { Ok "hosts.yaml names machine $Machine" } else { Bad "hosts.yaml on origin/workflow has no machine $Machine" }
    $drive = (Get-Item $Repo).PSDrive
    $freeGb = [int]($drive.Free / 1GB)
    if ($freeGb -ge 40) { Ok "${freeGb}G free on $($drive.Name):" } else { Warn "${freeGb}G free; runs skip below 40G (BW_MIN_FREE_GB)" }
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { Ok "installed: task '$TaskName'" } else { Warn 'not installed yet' }
    Write-Host "claude must be logged in for headless runs: run 'claude' once in this checkout if unsure."
    if ($script:Problems -eq 0) { Write-Host 'Ready.' } else { Write-Host "$($script:Problems) problem(s)." }
}

switch ($Command) {
    'check' { Check; if ($script:Problems) { exit 1 } }
    'install' {
        Check
        if ($script:Problems) { Write-Host 'Fix the problems above first.'; exit 1 }
        $arg = "-lc `"'$($script:UnixRepo)/tools/workflow/native/native-tick.sh' $Machine`""
        $action = New-ScheduledTaskAction -Execute $script:Bash -Argument $arg -WorkingDirectory $Repo
        $trigger = New-ScheduledTaskTrigger -Daily -At $At
        $rep = New-ScheduledTaskTrigger -Once -At $At -RepetitionInterval (New-TimeSpan -Hours $EveryHours) -RepetitionDuration (New-TimeSpan -Days 1)
        $trigger.Repetition = $rep.Repetition
        $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
            -ExecutionTimeLimit (New-TimeSpan -Hours 6) -MultipleInstances IgnoreNew
        $principal = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited
        Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings `
            -Principal $principal -Description 'BuildWarden scheduled native session (tools/workflow/native, FLOW-09)' -Force | Out-Null
        Write-Host "Installed '$TaskName': from $At every $EveryHours h for machine $Machine. Idle runs start no agent."
        Write-Host 'Logs: ~/.local/state/buildwarden-native/logs/ (in Git Bash)'
        Write-Host "Try one run now: Start-ScheduledTask -TaskName '$TaskName'"
    }
    'uninstall' {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
        Write-Host "Removed '$TaskName'. Logs and the gate stamp stay in ~/.local/state/buildwarden-native."
    }
}
