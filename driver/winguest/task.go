package winguest

import "fmt"

// StartupTaskCommand is the PowerShell that registers the warden-run startup
// task. A prepare step runs it once (from FirstLogonCommands); afterwards every
// boot of the image runs the task as SYSTEM. The task waits for the seed volume
// (a second disk can enumerate seconds after the -AtStartup trigger fires) and
// runs its warden-run.ps1.
//
// LENGTH IS LOAD-BEARING: this lands in a FirstLogonCommands <CommandLine>,
// capped at FirstLogonCommandLimit. The wait loop is delivered as a base64
// -EncodedCommand so no quoting has to survive XML plus the command wrapper,
// and the result uses single quotes only and no '&', so it is XML-safe.
//
// Hardening kept from earlier failures:
//   - The wait loop polls up to ~180 s: Get-Volume returns nothing at boot
//     until the seed disk enumerates.
//   - Battery settings: Register-ScheduledTask defaults block the task in a VM
//     whose power state is not reported as AC.
func StartupTaskCommand(seedLabel string) string {
	wait := fmt.Sprintf(
		"for($i=0;$i -lt 90;$i++){"+
			"$v=Get-Volume -FileSystemLabel '%s' -EA 0;"+
			"if($v.DriveLetter){&($v.DriveLetter+':\\warden-run.ps1');break};"+
			"Start-Sleep 2}",
		seedLabel)
	return fmt.Sprintf(
		"$a=New-ScheduledTaskAction -Execute 'powershell.exe' "+
			"-Argument '-NoProfile -ExecutionPolicy Bypass -EncodedCommand %s';"+
			"$s=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries "+
			"-DontStopIfGoingOnBatteries;"+
			"$t=New-ScheduledTaskTrigger -AtStartup;"+
			"Register-ScheduledTask -TaskName warden-run -Action $a -Trigger $t "+
			"-Settings $s -User SYSTEM -RunLevel Highest -Force",
		EncodedCommand(wait))
}
