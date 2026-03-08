package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	arch := "amd64"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		arch = strings.TrimSpace(os.Args[1])
	}

	rootDir, err := os.Getwd()
	if err != nil {
		fail(err)
	}

	iconPath := filepath.Join(rootDir, "build", "windows", "icon.ico")
	manifestPath := filepath.Join(rootDir, "build", "windows", "wails.exe.manifest")
	outputPath := filepath.Join(rootDir, fmt.Sprintf("rsrc_windows_%s.syso", arch))

	iconFile, err := os.Open(iconPath)
	if err != nil {
		fail(err)
	}
	defer iconFile.Close()

	icon, err := winres.LoadICO(iconFile)
	if err != nil {
		fail(err)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		fail(err)
	}

	var rs winres.ResourceSet
	if err := rs.SetIcon(winres.ID(1), icon); err != nil {
		fail(err)
	}
	if err := rs.Set(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault, manifestData); err != nil {
		fail(err)
	}

	vi := version.Info{}
	vi.SetFileVersion("0.1.0.0")
	vi.SetProductVersion("0.1.0.0")
	must(vi.Set(version.LangDefault, version.CompanyName, "mq83"))
	must(vi.Set(version.LangDefault, version.FileDescription, "Parquet Export Studio"))
	must(vi.Set(version.LangDefault, version.InternalName, "Parquet Export Studio"))
	must(vi.Set(version.LangDefault, version.OriginalFilename, "Parquet Export Studio.exe"))
	must(vi.Set(version.LangDefault, version.ProductName, "Parquet Export Studio"))
	must(vi.Set(version.LangDefault, version.Comments, "Cross-built Windows desktop app"))
	rs.SetVersionInfo(vi)

	out, err := os.Create(outputPath)
	if err != nil {
		fail(err)
	}
	defer out.Close()

	if err := rs.WriteObject(out, targetArch(arch)); err != nil {
		fail(err)
	}
}

func targetArch(arch string) winres.Arch {
	switch arch {
	case "386":
		return winres.ArchI386
	case "arm":
		return winres.ArchARM
	case "arm64":
		return winres.ArchARM64
	default:
		return winres.ArchAMD64
	}
}

func must(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
