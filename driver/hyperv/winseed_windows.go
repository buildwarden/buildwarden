//go:build windows

package hyperv

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// buildSeed assembles the per-build provisioning media the build VM boots with.
// For a Windows guest it stages the WARDEN seed + OOBE answer file, then
// packages them into a DVD ISO. Returns the seed ISO path (SeedVHDX unused).
//
// The Linux Hyper-V guest path (cloud-init CIDATA) is intentionally not built:
// the current milestone targets the Windows Server eval VHD.
//
// VALIDATION NOTE: the ISO packaging (IMAPI2), OOBE implicit-search discovery of
// the answer file off a DVD, and the seed getting a drive letter are the pieces
// that need a live boot against the real eval VHD to confirm.
func buildSeed(workDir, buildID string, isWindows bool, req *driver.BuildRequest) (seedISO, seedVHDX string, err error) {
	if !isWindows {
		return "", "", fmt.Errorf(
			"Hyper-V Linux build-guest seed not implemented; this milestone targets " +
				"the Windows Server eval VHD (use --guest-os windows)")
	}
	seedDir, err := stageSeedDir(workDir, runtime.GOARCH, req)
	if err != nil {
		return "", "", fmt.Errorf("buildSeed: %w", err)
	}
	seedISO = filepath.Join(workDir, "seed.iso")
	if err := makeSeedISOWindows(seedDir, seedISO, wardenSeedLabel); err != nil {
		return "", "", fmt.Errorf("buildSeed: %w", err)
	}
	return seedISO, "", nil
}

// makeSeedISOWindows builds an ISO from seedDir using the in-box IMAPI2 COM API
// (Windows' own CD/DVD imaging stack) via PowerShell. IMAPI2 needs no elevation
// and produces a Joliet image, so the seed's exact long filenames
// (warden-io.exe, warden-run.ps1, Autounattend.xml) survive and Windows assigns
// the mounted DVD a drive letter — unlike a hand-rolled level-1 ISO, which
// uppercases/8.3-mangles names and can surface with no drive letter.
//
// The result image's IStream is marshalled to disk with a tiny Add-Type helper
// (the documented way to persist an IMAPI2 image from PowerShell).
func makeSeedISOWindows(seedDir, outISO, label string) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @"
using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
public static class WardenIso {
  public static void Save(object comStream, string path) {
    IStream s = (IStream)comStream;
    System.Runtime.InteropServices.ComTypes.STATSTG stat; s.Stat(out stat, 1);
    int size = (int)stat.cbSize;
    byte[] buf = new byte[size];
    IntPtr read = Marshal.AllocHGlobal(8);
    try { s.Read(buf, size, read); File.WriteAllBytes(path, buf); }
    finally { Marshal.FreeHGlobal(read); }
  }
}
"@
$fsi = New-Object -ComObject IMAPI2FS.MsftFileSystemImage
$fsi.FileSystemsToCreate = 3   # ISO9660 (1) + Joliet (2)
$fsi.VolumeName = '%[1]s'
$fsi.Root.AddTree('%[2]s', $false)
$result = $fsi.CreateResultImage()
[WardenIso]::Save($result.ImageStream, '%[3]s')
'OK'
`, psEscapeSingle(label), psEscapeSingle(seedDir), psEscapeSingle(outISO))

	out, err := runPS(script)
	if err != nil {
		return fmt.Errorf("IMAPI2 ISO build: %w", err)
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("IMAPI2 ISO build: unexpected output: %q", strings.TrimSpace(out))
	}
	return nil
}
