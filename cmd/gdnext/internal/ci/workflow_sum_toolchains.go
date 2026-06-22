package ci

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
)

// doctorAuditRow mirrors cli.DoctorAuditRow's JSON shape so the
// summary can decode artefacts without importing the cli package
// (which would pull in gdnext's full dep graph).
type doctorAuditRow struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	Host    string `json:"host"`
	Library bool   `json:"library,omitempty"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Source  string `json:"source,omitempty"`
}

// toolchainRow is one rendered row of the supply-chain audit table:
// a single (slug, host, goos/goarch) cell with the downloaded
// archive's byte size + sha256 + source URL + on-disk path, plus a
// Changed flag set when the SHA differs from the prior run.
type toolchainRow struct {
	Slug    string
	Version string
	Host    string
	GOOS    string
	GOARCH  string
	Path    string
	Size    int64
	SHA256  string
	Source  string
	Changed bool
}

// collectToolchains downloads every `toolchain-audit-*` artefact of
// the given run, merges them, and (when prior is non-empty) marks
// rows whose SHA differs from the prior snapshot's same-key row.
func collectToolchains(repo string, runID int64, prior map[string]string) []toolchainRow {
	rows := fetchAuditArtifacts(repo, runID)
	out := make([]toolchainRow, 0, len(rows))
	for _, r := range rows {
		key := toolchainKey(r)
		out = append(out, toolchainRow{
			Slug:    r.Slug,
			Version: r.Version,
			Host:    r.Host,
			GOOS:    r.GOOS,
			GOARCH:  r.GOARCH,
			Path:    r.Path,
			Size:    r.Size,
			SHA256:  r.SHA256,
			Source:  r.Source,
			Changed: prior != nil && r.SHA256 != "" && prior[key] != "" && prior[key] != r.SHA256,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		if out[i].GOOS != out[j].GOOS {
			return out[i].GOOS < out[j].GOOS
		}
		return out[i].GOARCH < out[j].GOARCH
	})
	return out
}

// priorToolchainSHAs builds a (slug | host | goos/goarch) → sha256
// map from the prior run's artefacts, for the Changed diff.
func priorToolchainSHAs(repo string, runID int64) map[string]string {
	rows := fetchAuditArtifacts(repo, runID)
	out := map[string]string{}
	for _, r := range rows {
		out[toolchainKey(r)] = r.SHA256
	}
	return out
}

func toolchainKey(r doctorAuditRow) string {
	return r.Slug + "|" + r.Host + "|" + r.GOOS + "/" + r.GOARCH
}

// fetchAuditArtifacts lists every `toolchain-audit-*` artefact for
// the given run via gh api, downloads each via the archive_download_url,
// reads `toolchain-audit.json` from the zip, and merges rows.
func fetchAuditArtifacts(repo string, runID int64) []doctorAuditRow {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/runs/%d/artifacts", repo, runID),
		"-X", "GET", "-F", "per_page=100",
	).Output()
	if err != nil {
		return nil
	}
	var resp struct {
		Artifacts []struct {
			Name               string `json:"name"`
			ArchiveDownloadURL string `json:"archive_download_url"`
			Expired            bool   `json:"expired"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil
	}
	var rows []doctorAuditRow
	for _, a := range resp.Artifacts {
		if a.Expired || !strings.HasPrefix(a.Name, "toolchain-audit-") {
			continue
		}
		batch, err := downloadAuditArtifact(a.ArchiveDownloadURL)
		if err != nil {
			continue
		}
		rows = append(rows, batch...)
	}
	return rows
}

// downloadAuditArtifact fetches the zip at downloadURL via gh api,
// then unmarshals the toolchain-audit.json entry inside it.
func downloadAuditArtifact(downloadURL string) ([]doctorAuditRow, error) {
	// gh api accepts the full path after the host; strip the
	// "https://api.github.com" prefix.
	path := strings.TrimPrefix(downloadURL, "https://api.github.com/")
	body, err := exec.Command("gh", "api", path, "-X", "GET").Output()
	if err != nil {
		return nil, ghError(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != "toolchain-audit.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}
		var rows []doctorAuditRow
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	return nil, fmt.Errorf("toolchain-audit.json not in artefact zip")
}

func renderToolchainsMarkdown(w io.Writer, rows []toolchainRow) {
	fmt.Fprintln(w, `<h2 id="toolchains">Toolchains</h2>`)
	fmt.Fprintln(w)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No toolchain audit artefacts in the latest run._")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "| Toolchain | Version | Build Host | Path | Size | Changed | Source | SHA256 |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | ---: | :-: | --- | --- |")
	for _, r := range rows {
		ver := r.Version
		if ver == "" {
			ver = "—"
		}
		path := r.Path
		if path == "" {
			path = "—"
		} else {
			path = "`" + path + "`"
		}
		size := formatSize(r.Size)
		changed := ""
		if r.Changed {
			changed = "⚠"
		}
		source := r.Source
		if source != "" {
			source = fmt.Sprintf("[link](%s)", source)
		} else {
			source = "—"
		}
		sha := r.SHA256
		if sha == "" {
			sha = "—"
		} else if len(sha) > 12 {
			sha = "`" + sha[:12] + "…`"
		} else {
			sha = "`" + sha + "`"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			r.Slug, ver, r.Host, path, size, changed, source, sha)
	}
	fmt.Fprintln(w)
}

func formatSize(n int64) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.0fK", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2fG", float64(n)/(1024*1024*1024))
	}
}
