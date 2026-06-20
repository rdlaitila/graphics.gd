package product

import (
	"encoding/xml"
)

// Toolchain is the declarative record for one external program graphics.gd's
// build pipeline can drive (compiler, linker, packager, signer, ...). It
// names the tool, the version gdnext was built against, where to fetch it
// from per host, what build targets need it (Required), and which hosts
// can obtain it (Available).
//
// Toolchain itself is pure data — the runtime state (cached install path,
// download + extract logic, exec helpers) lives in
// cmd/gdnext/internal/tooling, which wraps these records to attach
// behaviour.
type Toolchain struct {
	XMLName          xml.Name                     `json:"-"                           xml:"toolchain"                       yaml:"-"`
	Slug             string                       `json:"slug"                        xml:"slug,attr"                       yaml:"slug"`
	Name             string                       `json:"name,omitempty"              xml:"name,omitempty"                  yaml:"name,omitempty"`
	Version          string                       `json:"version,omitempty"           xml:"version,omitempty"               yaml:"version,omitempty"`
	VersionFlags     []string                     `json:"version_flags,omitempty"     xml:"version_flags>flag,omitempty"    yaml:"version_flags,omitempty"`
	VersionPrefix    string                       `json:"version_prefix,omitempty"    xml:"version_prefix,omitempty"        yaml:"version_prefix,omitempty"`
	RequiredFor      string                       `json:"required_for,omitempty"      xml:"required_for,omitempty"          yaml:"required_for,omitempty"`
	AvailableHosts   []BuildHost                  `json:"available_hosts,omitempty"   xml:"available_hosts>host,omitempty" yaml:"available_hosts,omitempty"`
	Downloads        map[string]map[string]string `json:"downloads,omitempty"         xml:"-"                               yaml:"downloads,omitempty"`
	DownloadURL      string                       `json:"download_url,omitempty"      xml:"download_url,omitempty"          yaml:"download_url,omitempty"`
	DownloadARCH     map[string]string            `json:"download_arch,omitempty"     xml:"-"                               yaml:"download_arch,omitempty"`
	DownloadOS       map[string]string            `json:"download_os,omitempty"       xml:"-"                               yaml:"download_os,omitempty"`
	DownloadEXT      map[string]string            `json:"download_ext,omitempty"      xml:"-"                               yaml:"download_ext,omitempty"`
	DownloadHint     string                       `json:"download_hint,omitempty"     xml:"download_hint,omitempty"         yaml:"download_hint,omitempty"`
	Unzip            string                       `json:"unzip,omitempty"             xml:"unzip,omitempty"                 yaml:"unzip,omitempty"`
	Installations    map[string]string            `json:"installations,omitempty"     xml:"-"                               yaml:"installations,omitempty"`
	ConvertArguments map[string]string            `json:"convert_arguments,omitempty" xml:"-"                               yaml:"convert_arguments,omitempty"`
	IsApp            bool                         `json:"is_app,omitempty"            xml:"is_app,attr,omitempty"           yaml:"is_app,omitempty"`
	IsLibrary        bool                         `json:"is_library,omitempty"        xml:"is_library,attr,omitempty"       yaml:"is_library,omitempty"`
	DarwinUniversal  bool                         `json:"darwin_universal,omitempty"  xml:"darwin_universal,attr,omitempty" yaml:"darwin_universal,omitempty"`
}

// FindToolchainBySlug returns the matrix entry whose Slug matches slug.
func FindToolchainBySlug(slug string) (Toolchain, bool) {
	for _, t := range ToolchainMatrix {
		if t.Slug == slug {
			return t, true
		}
	}
	return Toolchain{}, false
}

// CanInstallOn reports whether this toolchain has a download / install
// path for the given host. An empty AvailableHosts list means the tool
// has no installer for any host (caller should treat as "not installable").
func (t Toolchain) CanInstallOn(host BuildHost) bool {
	for _, h := range t.AvailableHosts {
		if h.GOOS == host.GOOS && h.GOARCH == host.GOARCH {
			return true
		}
	}
	return false
}
