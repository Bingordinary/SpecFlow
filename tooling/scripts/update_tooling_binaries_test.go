package scripts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// The transport fixture serves one explicit release and records every request.
// Both script entry points run unchanged; no real release or network is used.
const releaseTransport = `package main
import ("fmt"; "os"; "path/filepath"; "strings")
func main() {
 name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
 if strings.HasPrefix(name, "specflowctl-") { fmt.Print(strings.Repeat("b", 64)); return }
 if name == "git" {
  if len(os.Args) > 1 && os.Args[1] == "ls-remote" && os.Getenv("TEST_MISSING_TAG") == "1" { os.Exit(2) }
  return
 }
 var url, out string
 if name == "curl" {
  for i := 1; i < len(os.Args); i++ {
   if os.Args[i] == "-o" { i++; out = os.Args[i] } else if strings.HasPrefix(os.Args[i], "https://") { url = os.Args[i] }
  }
 } else { url, out = os.Args[1], os.Args[2] }
 prefix := "https://github.com/Bingordinary/SpecFlow/releases/download/specflow-tooling-" + strings.Repeat("b", 12) + "/"
 if !strings.HasPrefix(url, prefix) { fmt.Fprintln(os.Stderr, "unexpected release URL:", url); os.Exit(2) }
 asset := strings.TrimPrefix(url, prefix)
 if asset == "" || filepath.Base(asset) != asset { os.Exit(2) }
 f, err := os.OpenFile(os.Getenv("TEST_REQUEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
 if err != nil { panic(err) }; fmt.Fprintln(f, asset); f.Close()
 if asset == "SHA256SUMS" && os.Getenv("TEST_MISSING_MANIFEST") == "1" { fmt.Fprintln(os.Stderr, "requested manifest unavailable"); os.Exit(22) }
 data, err := os.ReadFile(filepath.Join(os.Getenv("TEST_RELEASE_DIR"), asset))
 if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(22) }
 if err := os.WriteFile(out, data, 0600); err != nil { panic(err) }
}
`

func TestUpdateToolingBinariesReleaseIdentity(t *testing.T) {
	_, sourceFile, _, _ := runtime.Caller(0)
	scriptsDir := filepath.Dir(sourceFile)
	helperDir := t.TempDir()
	helperSource := filepath.Join(helperDir, "transport.go")
	writeReleaseTestFile(t, helperSource, []byte(releaseTransport), 0600)
	helperPath := filepath.Join(helperDir, "transport")
	if runtime.GOOS == "windows" {
		helperPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", helperPath, helperSource)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release transport: %v\n%s", err, output)
	}
	helperBytes, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatal(err)
	}
	native := runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		native += ".exe"
	}
	cross := "linux-amd64"
	if native == cross {
		cross = "darwin-arm64"
	}
	all := []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64.exe", "windows-arm64.exe"}
	for _, runner := range []string{"bash", "pwsh"} {
		t.Run(runner, func(t *testing.T) {
			if runner == "bash" && runtime.GOOS == "windows" {
				t.Skip("POSIX script requires a POSIX host")
			}
			runnerPath, err := exec.LookPath(runner)
			if err != nil {
				t.Skipf("%s runtime unavailable", runner)
			}
			for _, scenario := range []string{
				"changed_release_cross_only", "same_release_reuse", "native_and_cross",
				"corrupt_cached_binary", "cleared_cache", "manifest_unavailable", "tag_unavailable",
				"missing_checksum_entry", "duplicate_checksum_entry", "corrupt_download",
				"explicit_all", "current_only_reuse",
			} {
				t.Run(scenario, func(t *testing.T) {
					root := t.TempDir()
					binDir := filepath.Join(root, "tooling", "bin")
					remoteDir := filepath.Join(root, "release")
					mockDir := filepath.Join(root, "transport")
					for _, name := range []string{"git", "curl", "download"} {
						if runtime.GOOS == "windows" {
							name += ".exe"
						}
						writeReleaseTestFile(t, filepath.Join(mockDir, name), helperBytes, 0700)
					}
					selected := []string{cross}
					if scenario == "native_and_cross" {
						selected = append(selected, native)
					} else if scenario == "explicit_all" {
						selected = all
					} else if scenario == "current_only_reuse" {
						selected = []string{native}
					}
					cached, desired := map[string][]byte{}, map[string][]byte{}
					for _, suffix := range selected {
						name := "specflowctl-" + suffix
						desired[name] = []byte("new release: " + name)
						cached[name] = []byte("old release: " + name)
						if scenario == "same_release_reuse" {
							cached[name] = desired[name]
						} else if scenario == "current_only_reuse" {
							cached[name], desired[name] = helperBytes, helperBytes
						}
					}
					localSums := releaseTestSums(cached)
					if scenario == "corrupt_cached_binary" {
						localSums = releaseTestSums(desired)
					}
					if scenario != "cleared_cache" {
						for name, data := range cached {
							path := filepath.Join(binDir, name)
							writeReleaseTestFile(t, path, data, 0700)
							oldTime := time.Unix(946684800, 0)
							if err := os.Chtimes(path, oldTime, oldTime); err != nil {
								t.Fatal(err)
							}
						}
						writeReleaseTestFile(t, filepath.Join(binDir, "SHA256SUMS"), []byte(localSums), 0600)
					}
					remoteSums := releaseTestSums(desired)
					if scenario == "missing_checksum_entry" {
						remoteSums = releaseTestSums(map[string][]byte{"unselected": []byte("other")})
					} else if scenario == "duplicate_checksum_entry" {
						remoteSums += remoteSums
					}
					writeReleaseTestFile(t, filepath.Join(remoteDir, "SHA256SUMS"), []byte(remoteSums), 0600)
					for name, data := range desired {
						if scenario == "corrupt_download" {
							data = []byte("corrupt transport")
						}
						writeReleaseTestFile(t, filepath.Join(remoteDir, name), data, 0600)
					}
					writeReleaseTestFile(t, filepath.Join(root, "tooling", "fingerprint.txt"), []byte(strings.Repeat("b", 64)+"\n"), 0600)
					configured := selected
					if scenario == "explicit_all" || scenario == "current_only_reuse" {
						// Explicit mode flags must override this cross-only preference.
						configured = []string{cross}
					}
					writeReleaseTestFile(t, filepath.Join(root, "tooling", "platforms.txt"), []byte(strings.Join(configured, "\n")+"\n"), 0600)
					ext := ".sh"
					if runner == "pwsh" {
						ext = ".ps1"
					}
					scriptBytes, err := os.ReadFile(filepath.Join(scriptsDir, "update_tooling_binaries"+ext))
					if err != nil {
						t.Fatal(err)
					}
					script := filepath.Join(root, "tooling", "scripts", "update_tooling_binaries"+ext)
					writeReleaseTestFile(t, script, scriptBytes, 0700)
					args := []string{script}
					if runner == "pwsh" {
						wrapper := filepath.Join(root, "invoke.ps1")
						body := "$ErrorActionPreference = 'Stop'\nfunction Invoke-WebRequest { param([string]$Uri, [string]$OutFile) & download $Uri $OutFile; if ($LASTEXITCODE -ne 0) { throw 'Requested release download failed' } }\n& '" + strings.ReplaceAll(script, "'", "''") + "'"
						if scenario == "explicit_all" {
							body += " -All"
						} else if scenario == "current_only_reuse" {
							body += " -CurrentOnly"
						}
						writeReleaseTestFile(t, wrapper, []byte(body+"\n"), 0600)
						args = []string{"-NoProfile", "-File", wrapper}
					} else if scenario == "explicit_all" {
						args = append(args, "--all")
					} else if scenario == "current_only_reuse" {
						args = append(args, "--current-only")
					}
					cmd := exec.Command(runnerPath, args...)
					cmd.Dir = root
					logPath := filepath.Join(root, "requests.txt")
					cmd.Env = append(os.Environ(), "PATH="+mockDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_RELEASE_DIR="+remoteDir, "TEST_REQUEST_LOG="+logPath)
					if scenario == "manifest_unavailable" {
						cmd.Env = append(cmd.Env, "TEST_MISSING_MANIFEST=1")
					} else if scenario == "tag_unavailable" {
						cmd.Env = append(cmd.Env, "TEST_MISSING_TAG=1")
					}
					output, runErr := cmd.CombinedOutput()
					wantFailure := strings.Contains(scenario, "unavailable") || strings.Contains(scenario, "checksum_entry") || scenario == "corrupt_download"
					if (runErr != nil) != wantFailure {
						t.Fatalf("unexpected outcome: %v\n%s", runErr, output)
					}
					if wantFailure && strings.Contains(string(output), "already match") {
						t.Fatalf("failed release evidence reported success:\n%s", output)
					}
					for name, want := range desired {
						if wantFailure {
							want = cached[name]
						}
						actual, err := os.ReadFile(filepath.Join(binDir, name))
						if err != nil || string(actual) != string(want) {
							t.Fatalf("binary %s: err=%v; requested release bytes not installed, or failure changed cache", name, err)
						}
					}
					actualSums, err := os.ReadFile(filepath.Join(binDir, "SHA256SUMS"))
					wantSums := remoteSums
					if wantFailure {
						wantSums = localSums
					}
					if err != nil || string(actualSums) != wantSums {
						t.Fatalf("checksum table: %v; wrong release table or failed update changed it", err)
					}
					requests, _ := os.ReadFile(logPath)
					if scenario == "same_release_reuse" {
						if string(requests) != "SHA256SUMS\n" || !strings.Contains(string(output), "already match") {
							t.Fatalf("matching cache was not reused after release validation: requests=%s\n%s", requests, output)
						}
						info, _ := os.Stat(filepath.Join(binDir, "specflowctl-"+cross))
						if info.ModTime().Unix() != 946684800 {
							t.Fatal("matching binary was replaced")
						}
					} else if scenario == "current_only_reuse" || scenario == "tag_unavailable" {
						if len(requests) != 0 {
							t.Fatalf("unexpected release request: %s", requests)
						}
					} else if !strings.HasPrefix(string(requests), "SHA256SUMS\n") {
						t.Fatalf("requested-release manifest was not the first request: %s", requests)
					}
					if !wantFailure {
						for _, name := range []string{"specflowctl", "specflowctl.cmd"} {
							if _, err := os.Stat(filepath.Join(binDir, name)); err != nil {
								t.Fatalf("successful updater did not write launcher %s: %v", name, err)
							}
						}
					}
				})
			}
		})
	}
}

func releaseTestSums(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	return sums.String()
}

func writeReleaseTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
