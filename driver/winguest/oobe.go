package winguest

import "fmt"

// OOBEConfig parameterizes the oobeSystem-only answer file used to take a
// generalized image (Microsoft's eval VHD, or warden's sysprepped ISO install)
// through OOBE unattended.
type OOBEConfig struct {
	// ProcArch is the unattend processorArchitecture ("amd64" or "arm64").
	ProcArch string
	// AdminPassword is the built-in Administrator password, random per use. It
	// is only used for the single autologon that runs FirstLogonCommand.
	AdminPassword string
	// FirstLogonCommand runs once at that autologon; it must stay under
	// FirstLogonCommandLimit.
	FirstLogonCommand string
	// Description labels the FirstLogonCommands entry.
	Description string
}

// OOBEUnattend renders the answer file: auto-answer locale, set and autologon
// the built-in Administrator once, hide every OOBE screen, run
// FirstLogonCommand. Windows finds it by implicit search (unattend.xml or
// Autounattend.xml at the root of attached media).
func OOBEUnattend(cfg OOBEConfig) string {
	arch := cfg.ProcArch
	if arch == "" {
		arch = "amd64"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend"
 xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core"
      processorArchitecture="%[1]s" publicKeyToken="31bf3856ad364e35"
      language="neutral" versionScope="nonSxS">
      <InputLocale>en-US</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-Shell-Setup"
      processorArchitecture="%[1]s" publicKeyToken="31bf3856ad364e35"
      language="neutral" versionScope="nonSxS">
      <UserAccounts>
        <AdministratorPassword>
          <Value>%[2]s</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <AutoLogon>
        <Enabled>true</Enabled>
        <LogonCount>1</LogonCount>
        <Username>Administrator</Username>
        <Password>
          <Value>%[2]s</Value>
          <PlainText>true</PlainText>
        </Password>
      </AutoLogon>
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideLocalAccountScreen>true</HideLocalAccountScreen>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <NetworkLocation>Work</NetworkLocation>
        <ProtectYourPC>3</ProtectYourPC>
      </OOBE>
      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add">
          <Order>1</Order>
          <CommandLine>%[3]s</CommandLine>
          <Description>%[4]s</Description>
        </SynchronousCommand>
      </FirstLogonCommands>
      <TimeZone>UTC</TimeZone>
    </component>
  </settings>
</unattend>
`, arch, XMLText(cfg.AdminPassword), XMLText(cfg.FirstLogonCommand),
		XMLText(cfg.Description))
}

// GeneralizeCommand is the last FirstLogonCommand of an ISO install: it
// turns the installed disk into a generalized base like Microsoft's eval VHD.
// It deletes the answer file Setup cached under Panther (else the next OOBE
// would reuse it, autologon and generalize again, instead of reading the
// prepare seed's), enables the built-in Administrator that prepare's OOBE
// logs on as, disables installUser, and runs sysprep, which powers off.
// Without SkipRearm, so the grace clock starts again at each specialize.
func GeneralizeCommand(installUser string) string {
	script := fmt.Sprintf(
		"$p=\"$env:windir\\Panther\";"+
			"Remove-Item \"$p\\unattend.xml\",\"$p\\Autounattend.xml\" -Force -EA 0;"+
			"net user Administrator /active:yes;"+
			"net user '%s' /active:no;"+
			"& \"$env:windir\\System32\\Sysprep\\sysprep.exe\" "+
			"/generalize /oobe /shutdown /quiet", installUser)
	return "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand " +
		EncodedCommand(script)
}

// SeedScriptCommand is a FirstLogonCommand that finds script at the root of
// any filesystem drive (the attached seed) and runs it. The scan is delivered
// as a base64 -EncodedCommand, so no quoting has to survive XML and cmd.
func SeedScriptCommand(script string) string {
	scan := fmt.Sprintf(`gdr -PSProvider FileSystem|`+
		`%%{$p=$_.Root+'%s';if(Test-Path $p){& $p;break}}`, script)
	return "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand " +
		EncodedCommand(scan)
}
