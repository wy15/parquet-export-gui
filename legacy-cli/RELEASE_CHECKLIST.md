# Win7/8 Legacy CLI Release Checklist

## Build

- Confirm the toolchain is Go `1.20.x`
- Run `LEGACY_GO_BIN=/path/to/go1.20.x/bin/go ./scripts/build_legacy_windows_cli_from_macos.sh`
- Verify the output file exists at `dist/windows7-cli/Parquet Export Studio Legacy CLI.exe`
- Record the exact Go version used for the build

## Smoke Test

- Launch the CLI on a clean Windows 7 or Windows 8 machine
- Confirm the interactive menu renders correctly in `cmd.exe`
- Run a connection test for each backend you plan to support in that release
- Export a small table and confirm the output `.parquet` file is created
- Re-run export to verify the rename/overwrite conflict policy still behaves correctly

## Data Validation

- Open the generated Parquet file with a modern reader on another machine
- Confirm row count matches the source table
- Spot-check string, integer, float, boolean, binary, and datetime columns
- For MaxCompute, verify schema and partition inputs still target the intended table

## Packaging

- Rename the artifact to include version and architecture if needed
- Generate checksum files for the final `.exe`
- Publish a short compatibility note stating the binary is built with Go `1.20.x`
- Keep the exact build command and checksum alongside the release notes

## Regression Watch

- Note that the legacy line uses older Arrow, pgx, mysql, and ODPS dependencies
- Re-test Win7/8 after any dependency bump, even if the module still says `go 1.20`
- Avoid merging GUI-only changes into `legacy-cli` without re-running the CLI smoke test
