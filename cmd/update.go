package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultUpdateRepo = "dragonsecurity-incubating/vm-info"

var (
	updateCheck   bool
	updateYes     bool
	updateForce   bool
	updateVersion string
	updatePre     bool
	updateRepo    string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update vm-info to the latest GitHub release",
	Long: `Replace the running binary with the latest release from GitHub.

Picks the asset matching the running OS/arch, verifies it against the
release's SHA256SUMS, and atomically replaces the current binary on
Unix-like systems (rename over an open executable is safe on Linux and
macOS). Skips the swap when:

  - the binary path looks like a 'go run' temp build
  - the requested version equals the current Version (use --force to
    reinstall the same tag)
  - the user answers no at the confirmation prompt (use --yes / -y to
    skip)`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	self, err := selfPath()
	if err != nil {
		return err
	}

	tag := strings.TrimSpace(updateVersion)
	rel, err := fetchRelease(updateRepo, tag, updatePre)
	if err != nil {
		return err
	}

	if !updateForce && rel.TagName != "" && rel.TagName == Version {
		fmt.Fprintf(out, "Already on %s\n", Version)
		return nil
	}

	asset, sumsAsset, err := pickAssets(rel)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Current: %s\nLatest:  %s\nAsset:   %s\n", Version, rel.TagName, asset.Name)
	if updateCheck {
		return nil
	}

	if !updateYes {
		fmt.Fprintf(out, "\nReplace %s with %s? [y/N]: ", self, rel.TagName)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	expected, err := fetchExpectedChecksum(sumsAsset, asset.Name)
	if err != nil {
		return err
	}

	tmpArchive, err := downloadToTemp(asset.URL, "vm-info-update-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmpArchive)

	got, err := sha256File(tmpArchive)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset.Name, expected, got)
	}
	fmt.Fprintln(out, "Checksum verified.")

	innerName := strings.TrimSuffix(asset.Name, ".tar.gz")
	tmpBin, err := extractBinary(tmpArchive, innerName, filepath.Dir(self))
	if err != nil {
		return err
	}
	defer os.Remove(tmpBin) // best-effort; gone after rename

	if err := os.Chmod(tmpBin, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	if err := os.Rename(tmpBin, self); err != nil {
		return fmt.Errorf("replace %s: %w (need elevated permissions?)", self, err)
	}

	fmt.Fprintf(out, "Updated %s to %s.\n", self, rel.TagName)
	return nil
}

// --- self-path detection -------------------------------------------------

func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate self: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}
	if isGoRunPath(exe) {
		return "", fmt.Errorf("refusing to self-update a 'go run' temp build (%s); install vm-info first", exe)
	}
	return exe, nil
}

func isGoRunPath(p string) bool {
	tmp := os.TempDir()
	return strings.HasPrefix(p, tmp) && (strings.Contains(p, "/go-build") || strings.Contains(p, `\go-build`))
}

// --- GitHub release lookup ----------------------------------------------

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Prerelease bool      `json:"prerelease"`
	Draft      bool      `json:"draft"`
	Assets     []ghAsset `json:"assets"`
}

// fetchRelease returns one release. tag="" picks the newest non-prerelease
// (or any if includePre is true).
func fetchRelease(repo, tag string, includePre bool) (ghRelease, error) {
	if tag != "" {
		var rel ghRelease
		if err := ghGet(fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag), &rel); err != nil {
			return ghRelease{}, fmt.Errorf("fetch release %s: %w", tag, err)
		}
		return rel, nil
	}
	if !includePre {
		var rel ghRelease
		if err := ghGet(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), &rel); err != nil {
			return ghRelease{}, fmt.Errorf("fetch latest release: %w", err)
		}
		return rel, nil
	}
	var all []ghRelease
	if err := ghGet(fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", repo), &all); err != nil {
		return ghRelease{}, fmt.Errorf("list releases: %w", err)
	}
	for _, r := range all {
		if r.Draft {
			continue
		}
		return r, nil
	}
	return ghRelease{}, errors.New("no releases found")
}

func pickAssets(rel ghRelease) (binary, sums ghAsset, err error) {
	want := fmt.Sprintf("vm-info-%s-%s-%s.tar.gz", rel.TagName, runtime.GOOS, runtime.GOARCH)
	for _, a := range rel.Assets {
		switch a.Name {
		case want:
			binary = a
		case "SHA256SUMS":
			sums = a
		}
	}
	if binary.URL == "" {
		return binary, sums, fmt.Errorf("no asset %q in release %s", want, rel.TagName)
	}
	if sums.URL == "" {
		return binary, sums, fmt.Errorf("no SHA256SUMS asset in release %s", rel.TagName)
	}
	return binary, sums, nil
}

// --- HTTP helpers --------------------------------------------------------

var httpClient = &http.Client{Timeout: 60 * time.Second}

func ghGet(url string, out any) error {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "vm-info/"+Version)
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func downloadToTemp(url, pattern string) (string, error) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set("User-Agent", "vm-info/"+Version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if closeErr != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close download: %w", closeErr)
	}
	return f.Name(), nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchExpectedChecksum downloads SHA256SUMS and returns the hex digest for
// the named file (lines look like "<sha256>  <filename>").
func fetchExpectedChecksum(sumsAsset ghAsset, fileName string) (string, error) {
	tmp, err := downloadToTemp(sumsAsset.URL, "vm-info-sums-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	f, err := os.Open(tmp)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// "filename" may have a leading "*" for binary mode (sha256sum -b)
		got := strings.TrimPrefix(fields[len(fields)-1], "*")
		if got == fileName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no entry for %s in SHA256SUMS", fileName)
}

// extractBinary writes the tarball entry named `inner` into dir as a
// fresh temp file and returns its path. The temp file lives in the same
// directory as the target so os.Rename is atomic on the same filesystem.
func extractBinary(archivePath, inner, dir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s not found in archive", inner)
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(hdr.Name) != inner {
			continue
		}
		out, err := os.CreateTemp(dir, ".vm-info-update-*")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
			return "", err
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "only print the available release; don't download or replace")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "skip the confirmation prompt")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "reinstall even if the target version equals the current one")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "specific release tag to install (default: latest)")
	updateCmd.Flags().BoolVar(&updatePre, "pre", false, "include pre-releases when picking 'latest'")
	updateCmd.Flags().StringVar(&updateRepo, "repo", defaultUpdateRepo, "GitHub repo (owner/name)")
	rootCmd.AddCommand(updateCmd)
}
