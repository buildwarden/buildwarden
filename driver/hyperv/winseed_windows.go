//go:build windows

package hyperv

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

// buildSeed assembles the per-build seed ISO the build VM boots with: the
// WARDEN volume the prepared image's warden-run task looks for. Returns the
// seed ISO path (SeedVHDX unused). A Linux Hyper-V guest is not built yet.
func buildSeed(
	workDir, buildID string, isWindows bool, req *driver.BuildRequest,
) (seedISO, seedVHDX string, err error) {
	if !isWindows {
		return "", "", fmt.Errorf(
			"Hyper-V Linux build-guest seed not implemented; this milestone targets " +
				"Windows guests (use --guest-os windows)")
	}
	seedDir, err := stageSeedDir(workDir, runtime.GOARCH, req)
	if err != nil {
		return "", "", fmt.Errorf("buildSeed: %w", err)
	}
	seedISO = filepath.Join(workDir, "seed.iso")
	if err := makeSeedISOWindows(seedDir, seedISO, winguest.SeedLabel); err != nil {
		return "", "", fmt.Errorf("buildSeed: %w", err)
	}
	return seedISO, "", nil
}

// buildPrepareSeed stages the prepare seed in workDir and packages it as an
// ISO labelled winguest.PrepareSeedLabel. It returns the ISO and the staged
// directory (whose files are the prepared layer's recorded inputs).
func buildPrepareSeed(workDir string) (seedISO, seedDir string, err error) {
	seedDir = filepath.Join(workDir, "prepare-seed")
	if err := stagePrepareSeedDir(seedDir, runtime.GOARCH); err != nil {
		return "", "", err
	}
	seedISO = filepath.Join(workDir, "prepare-seed.iso")
	if err := makeSeedISOWindows(seedDir, seedISO, winguest.PrepareSeedLabel); err != nil {
		return "", "", err
	}
	return seedISO, seedDir, nil
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
		return fmt.Errorf(
			"IMAPI2 ISO build: unexpected output: %q", strings.TrimSpace(out))
	}
	return nil
}
