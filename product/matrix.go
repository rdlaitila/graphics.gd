package product

// GOOSMatrix is the canonical list of operating systems that graphics.gd
// recognizes for build and host purposes.
var GOOSMatrix = []string{
	GOOSLinux,
	GOOSUbuntu,
	GOOSArch,
	GOOSDebian,
	GOOSNix,
	GOOSMusl,
	GOOSWindows,
	GOOSWin,
	GOOSDarwin,
	GOOSMacos,
	GOOSIOS,
	GOOSIPhone,
	GOOSAndroid,
	GOOSMetaQuest,
	GOOSMeta,
	GOOSQuest,
	GOOSJS,
	GOOSWeb,
	GOOSBrowser,
}

var (
	GOOSLinux     = "linux"
	GOOSUbuntu    = "ubuntu" // remapped to GOOSLinux
	GOOSArch      = "arch"   // remapped to GOOSLinux
	GOOSDebian    = "debian" // remapped to GOOSLinux
	GOOSNix       = "nix"    // remapped to GOOSLinux
	GOOSMusl      = "musl"   // remapped to GOOSLinux
	GOOSWindows   = "windows"
	GOOSWin       = "win" // remapped to GOOSWindows
	GOOSDarwin    = "darwin"
	GOOSMacos     = "macos" // remapped to GOOSDarwin
	GOOSIOS       = "ios"
	GOOSIPhone    = "iphone" // remapped to GOOSIOS
	GOOSAndroid   = "android"
	GOOSMetaQuest = "metaquest"
	GOOSQuest     = "quest" // remapped to GOOSMetaQuest
	GOOSMeta      = "meta"  // remapped to GOOSMetaQuest
	GOOSJS        = "js"
	GOOSWeb       = "web"     // remapped to GOOSJS
	GOOSBrowser   = "browser" // remapped to GOOSJS
	GOOSWasm      = "wasm"    // remapped to GOOSJS
)

// GOARCHMatrix is the canonical list of CPU architectures that graphics.gd
// recognizes for build and host purposes.
var GOARCHMatrix = []string{
	GOARCHAmd64,
	GOARCHArm64,
	GOARCHWasm,
}

var (
	GOARCHAmd64 = "amd64"
	GOARCHArm64 = "arm64"
	GOARCHWasm  = "wasm"
)

// GOOSArchDefaults maps each GOOS to its default GOARCH, used when the target
// architecture is not explicitly set.
var GOOSArchDefaults = map[string]string{
	GOOSAndroid:   GOARCHArm64,
	GOOSMetaQuest: GOARCHArm64,
	GOOSIOS:       GOARCHArm64,
	GOOSDarwin:    GOARCHArm64,
	GOOSWindows:   GOARCHAmd64,
	GOOSLinux:     GOARCHAmd64,
	GOOSWeb:       GOARCHWasm,
}

// GOOSRemaps defines the canonical remapping of certain GOOS values to their
// internal equivalents, ensuring consistent handling across the platform logic.
var GOOSRemaps = map[string]string{
	GOOSMacos:   GOOSDarwin,
	GOOSIPhone:  GOOSIOS,
	GOOSWeb:     GOOSJS,
	GOOSBrowser: GOOSJS,
	GOOSWasm:    GOOSJS,
	GOOSWin:     GOOSWindows,
	GOOSUbuntu:  GOOSLinux,
	GOOSArch:    GOOSLinux,
	GOOSDebian:  GOOSLinux,
	GOOSNix:     GOOSLinux,
	GOOSQuest:   GOOSMetaQuest,
	GOOSMeta:    GOOSMetaQuest,
	GOOSMusl:    GOOSLinux,
}

// GOOSAliasLinkMode pins certain GOOS aliases to a specific LinkMode
// regardless of --link. "musl" implies LibGodot.
var GOOSAliasLinkMode = map[string]LinkMode{
	GOOSMusl: LibGodot,
}

// HostMatrix is the canonical list of platforms that can act as hosts
// for building graphics.gd projects.
var HostMatrix = []BuildHost{
	HostLinuxAmd64,
	HostWindowsAmd64,
	HostDarwinAmd64,
	HostDarwinArm64,
}

var (
	HostLinuxAmd64 = BuildHost{
		GOOS:   GOOSLinux,
		GOARCH: GOARCHAmd64,
	}
	HostWindowsAmd64 = BuildHost{
		GOOS:   GOOSWindows,
		GOARCH: GOARCHAmd64,
	}
	HostDarwinAmd64 = BuildHost{
		GOOS:   GOOSDarwin,
		GOARCH: GOARCHAmd64,
	}
	HostDarwinArm64 = BuildHost{
		GOOS:   GOOSDarwin,
		GOARCH: GOARCHArm64,
	}
)

// PlatformMatrix is the canonical, ordered list of every (GOOS, GOARCH) pair
// graphics.gd supports as either a host (where gdnext runs) or a build
// target (where the engine + a graphics.gd app can run).
var PlatformMatrix = []Platform{
	PlatformLinuxAmd64,
	PlatformLinuxArm64,
	PlatformWindowsAmd64,
	PlatformWindowsArm64,
	PlatformDarwinAmd64,
	PlatformDarwinArm64,
	PlatformIOSArm64,
	PlatformAndroidArm64,
	PlatformAndroidAmd64,
	PlatformMetaQuest,
	PlatformWebWasm,
}

var (
	// --- Linux ----------------------------------------------------------
	PlatformLinuxAmd64 = Platform{
		Title:      "Linux x86_64",
		GOOS:       GOOSLinux,
		GOARCH:     GOARCHAmd64,
		Aliases:    []string{GOOSUbuntu, GOOSArch, GOOSDebian, GOOSNix, "ublue/bazzite"},
		Kind:       Host | Target,
		Status:     Supported | Stable,
		LinkModes:  GDExtension | LibGodot,
		BuildHosts: HostMatrix,
		PlayHosts:  []BuildHost{HostLinuxAmd64},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
		Notes:      "libgodot mode (--link=libgodot or GOOS=musl alias) currently fetches the .musl. artefact",
	}
	PlatformLinuxArm64 = Platform{
		Title:      "Linux ARM64",
		GOOS:       GOOSLinux,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
		Notes:      "cross-compiled from any host via zig; libgodot mode pending an arm64 artefact",
	}
	// --- Windows --------------------------------------------------------
	PlatformWindowsAmd64 = Platform{
		Title:      "Windows x86_64",
		GOOS:       GOOSWindows,
		GOARCH:     GOARCHAmd64,
		Aliases:    []string{GOOSWin},
		Kind:       Host | Target,
		Status:     Supported | Stable,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		PlayHosts:  []BuildHost{HostLinuxAmd64},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
	}
	PlatformWindowsArm64 = Platform{
		Title:      "Windows ARM64",
		GOOS:       GOOSWindows,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
	}
	// --- macOS ----------------------------------------------------------
	PlatformDarwinAmd64 = Platform{
		Title:      "macOS x86_64",
		GOOS:       GOOSDarwin,
		GOARCH:     GOARCHAmd64,
		Aliases:    []string{GOOSMacos},
		Kind:       Host | Target,
		Status:     Supported | Quirky,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"metal", "opengl3", "gl_compatibility"},
		Notes:      "exports as a universal .app alongside arm64",
		Quirks:     []Quirk{QuirkWindowsDarwinBuildAccessDenied},
	}
	PlatformDarwinArm64 = Platform{
		Title:      "macOS Apple Silicon",
		GOOS:       GOOSDarwin,
		GOARCH:     GOARCHArm64,
		Kind:       Host | Target,
		Status:     Supported | Quirky,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"metal", "opengl3", "gl_compatibility"},
		Notes:      "produces a universal .app; lipo + codesign need a darwin host",
		Quirks:     []Quirk{QuirkWindowsDarwinBuildAccessDenied},
	}
	// --- iOS ------------------------------------------------------------
	PlatformIOSArm64 = Platform{
		Title:      "iOS ARM64",
		GOOS:       GOOSIOS,
		GOARCH:     GOARCHArm64,
		Aliases:    []string{GOOSIPhone},
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: []BuildHost{HostDarwinAmd64, HostDarwinArm64, HostLinuxAmd64},
		BuildTools: append(SharedToolchains, []Toolchain{ToolchainLLVM}...),
		Renderers:  []string{"metal", "gl_compatibility"},
		Notes:      "requires llvm; signing needs a macOS host + Apple cert",
	}
	// --- Android --------------------------------------------------------
	PlatformAndroidArm64 = Platform{
		Title:      "Android ARM64",
		GOOS:       GOOSAndroid,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported | Stable,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan", "gl_compatibility"},
	}
	PlatformAndroidAmd64 = Platform{
		Title:      "Android x86_64",
		GOOS:       GOOSAndroid,
		GOARCH:     GOARCHAmd64,
		Kind:       Target,
		Status:     Supported | Quirky,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan", "gl_compatibility"},
		Notes:      "primarily emulator or desktop android targeted (ex: waydroid)",
	}
	// --- Meta Quest (Android variant with OpenXR loader) ---------------
	PlatformMetaQuest = Platform{
		Title:      "Meta Quest",
		GOOS:       GOOSMetaQuest,
		GOARCH:     GOARCHArm64,
		Aliases:    []string{GOOSQuest, GOOSMeta},
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan"},
		Notes:      "Android profile with GodotVR + OpenXR injected into the apk",
	}
	// --- Web (WebAssembly + WebGL) -------------------------------------
	PlatformWebWasm = Platform{
		Title:      "Web (WebAssembly)",
		GOOS:       GOOSJS,
		GOARCH:     GOARCHWasm,
		Aliases:    []string{GOOSWeb, GOOSBrowser, GOOSWasm},
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: HostMatrix,
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"gl_compatibility"},
		Notes:      "COEP/COOP headers required when serving",
	}
)

// ToolchainMatrix is the canonical, ordered list of every external program
// graphics.gd's build pipeline can drive. Order matters: it's the order
// `gdnext toolchain {list,doctor}` use when rendering their tables and the
// order `gdnext toolchain install` (no args) walks. Keep base-required
// tools (godot, go, zig) first.
var ToolchainMatrix = []Toolchain{
	ToolchainGodot,
	ToolchainGo,
	ToolchainZig,
	ToolchainLLVM,
	ToolchainADB,
	ToolchainApkSigner,
	ToolchainAAPT2,
	ToolchainApkTool,
	ToolchainBundleTool,
	ToolchainAndroidJar,
	ToolchainUPX,
	ToolchainVPK,
	ToolchainLibGodot,
	ToolchainLibGodotEditor,
	ToolchainLDD,
}

// SharedToolchains lists the toolchains that are required for all builds,
// typically the base tools like godot, go, and zig.
var SharedToolchains = []Toolchain{
	ToolchainGodot,
	ToolchainGo,
	ToolchainZig,
}

// AndroidToolchains lists the toolchains required for building Android targets.
var AndroidToolchains = []Toolchain{
	ToolchainADB,
	ToolchainApkSigner,
	ToolchainAAPT2,
	ToolchainApkTool,
	ToolchainBundleTool,
	ToolchainAndroidJar,
}

// LibGodotToolchains is what LibGodot-mode builds need on top of
// SharedToolchains. The doctor / install loop appends these for every
// Platform whose LinkModes includes LibGodot. (ldd lives in setup.go's
// musl-host detection and is not listed here.)
var LibGodotToolchains = []Toolchain{
	ToolchainLLVM,
	ToolchainLibGodot,
	ToolchainLibGodotEditor,
}

var (
	ToolchainGodot = Toolchain{
		Slug:           "godot",
		Name:           "godot",
		Version:        "4.6.2",
		VersionFlags:   []string{"--version"},
		VersionPrefix:  "4.6.",
		RequiredFor:    "graphics",
		AvailableHosts: HostMatrix,
		DownloadHint:   "https://godotengine.org/download",
		DownloadURL:    "https://github.com/godotengine/godot/releases/download/$(VERSION)-stable/Godot_v$(VERSION)-stable_$(OS).zip",
		DownloadOS:     map[string]string{"windows": "win64.exe", "linux": "linux.$(ARCH)", "darwin": "macos.universal"},
		DownloadARCH:   map[string]string{"amd64": "x86_64", "arm64": "arm64"},
		Unzip:          "Godot_v$(VERSION)-stable_$(OS)",
		IsApp:          true,
		ConvertArguments: map[string]string{
			"-v":       "--verbose",
			"-x":       "",
			"-gcflags": "",
		},
		KnownChecksums: []string{
			"sha256:666b2a64e4b5c59db0e4974605b888eb72eb7d4e60e870d2be6cc19727b50807", // darwin/arm64
			"sha256:30e6b6d141f0cd5bebd629ad1d0ef1324e60091bb20662d026b402ba58c59937", // linux/amd64
			"sha256:14293422efb54b24a51f79d4cb55ab4001ef3d936e064a6c8af32e1f984024be", // windows/amd64
		},
	}
	ToolchainGo = Toolchain{
		Slug:           "go",
		Name:           "go",
		Version:        "1.26.0",
		VersionFlags:   []string{"version"},
		VersionPrefix:  "go version go1.26.",
		RequiredFor:    "compiling",
		AvailableHosts: HostMatrix,
		DownloadHint:   "https://go.dev/dl/",
		ConvertArguments: map[string]string{
			"--verbose": "-v",
		},
	}
	ToolchainZig = Toolchain{
		Slug:           "zig",
		Name:           "zig",
		Version:        "0.15.2",
		VersionFlags:   []string{"version"},
		RequiredFor:    "cross-compiling",
		AvailableHosts: HostMatrix,
		DownloadHint:   "https://ziglang.org/download/",
		DownloadURL:    "https://ziglang.org/download/$(VERSION)/zig-$(ARCH)-$(OS)-$(VERSION)$(EXT)",
		DownloadOS:     map[string]string{"windows": "windows", "darwin": "macos", "linux": "linux"},
		DownloadARCH:   map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
		DownloadEXT:    map[string]string{"windows": ".zip", "darwin": ".tar.xz", "linux": ".tar.xz"},
		KnownChecksums: []string{
			"sha256:3cc2bab367e185cdfb27501c4b30b1b0653c28d9f73df8dc91488e66ece5fa6b", // darwin/arm64
			"sha256:02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239", // linux/amd64
			"sha256:3a0ed1e8799a2f8ce2a6e6290a9ff22e6906f8227865911fb7ddedc3cc14cb0c", // windows/amd64
		},
	}
	ToolchainLLVM = Toolchain{
		Slug:           "llvm",
		Name:           "llvm",
		Version:        "21.1.8",
		VersionFlags:   []string{"clang", "--version"},
		VersionPrefix:  "clang version 21.",
		RequiredFor:    "linking iOS builds",
		AvailableHosts: HostMatrix,
		DownloadURL:    "https://release.graphics.gd/llvm.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:    map[string]string{"linux": "", "windows": ".exe", "darwin": ""},
		KnownChecksums: []string{
			"sha256:af69bb5c9cc71f8563bd452bf79755ae854819c501b6f3b89a5c873d45bd1dc4", // darwin/arm64
			"sha256:928da8c2a12f1085052cd04f2877c2ed25a1f9b2492b0e654a65c39cffdd3167", // linux/amd64
			"sha256:af59510bd670c4b2d190e5e6318b9ff4fc05736a1aa60874cfb05eaed8fd5d8d", // windows/amd64
		},
	}
	ToolchainADB = Toolchain{
		Slug:            "adb",
		Name:            "adb",
		Version:         "1.0.41",
		VersionFlags:    []string{"--version"},
		VersionPrefix:   "Android Debug Bridge version 1.0.41",
		RequiredFor:     "launching the project on a connected android device",
		AvailableHosts:  HostMatrix,
		DownloadURL:     "https://release.graphics.gd/adb.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:     map[string]string{"linux": "", "windows": ".zip", "darwin": ""},
		DarwinUniversal: true,
		KnownChecksums: []string{
			"sha256:3e8fb3a897103e32588f863e56fc7eaffdf16a542c4e690cc4326e4b766827a6", // linux/amd64
			"sha256:fbd3fcf03b91e7dafa3a8cfa54823ea91a3d9e7045e7bf8a07b8822167400e5c", // windows/amd64
		},
	}
	ToolchainApkSigner = Toolchain{
		Slug:           "apksigner",
		Name:           "apksigner",
		Version:        "0.9",
		VersionFlags:   []string{"--version"},
		RequiredFor:    "building the .apk",
		AvailableHosts: HostMatrix,
		DownloadURL:    "https://release.graphics.gd/apksigner.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:    map[string]string{"linux": "", "windows": ".exe", "darwin": ""},
		KnownChecksums: []string{
			"sha256:3b81d734787ac083dc7aa7951bfe2aca6823590a8064dae59fe898f2b25c866a", // darwin/arm64
			"sha256:8441bed7018d08af0d18653e9875290550ccc27f9f0f7768f12a59e28d60fbe0", // linux/amd64
			"sha256:68bbc75644892dced984c90582b55a11052d48cb235f92bbbe8127afdfdca81d", // windows/amd64
		},
	}
	ToolchainAAPT2 = Toolchain{
		Slug:            "aapt2",
		Name:            "aapt2",
		Version:         "2.19-android-13.0.0_r6",
		VersionFlags:    []string{"version"},
		VersionPrefix:   "Android Asset Packaging Tool (aapt) 2.",
		RequiredFor:     "converting the exported .apk into an .aab",
		AvailableHosts:  HostMatrix,
		DownloadURL:     "https://release.graphics.gd/aapt2.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:     map[string]string{"linux": "", "windows": ".exe", "darwin": ""},
		DarwinUniversal: true,
		KnownChecksums: []string{
			"sha256:5da28e9fb72bfd3452c56f21ddd084787e1833cdf27e99b7588bbd4aba6585ca", // darwin/arm64
			"sha256:9dd86ae76ae12d263672c4c454f17b30e42bb9792b3e2c0ce9d68b33fd5a7d37", // linux/amd64
			"sha256:b39c3ec3f8cba2ce36749546802a60141f879c950b308a92c759daf5cab2c843", // windows/amd64
		},
	}
	ToolchainApkTool = Toolchain{
		Slug:           "apktool",
		Name:           "apktool",
		Version:        "2.12.1",
		VersionFlags:   []string{"v"},
		VersionPrefix:  "2.12.1-",
		RequiredFor:    "converting the exported .apk into an .aab",
		AvailableHosts: HostMatrix,
		DownloadURL:    "https://release.graphics.gd/apktool.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:    map[string]string{"linux": "", "windows": ".exe", "darwin": ""},
		KnownChecksums: []string{
			"sha256:121531c7ee189a3d4e8ebd54c0872c2a2e6a9709a286249449441e0004b92798", // darwin/arm64
			"sha256:cf6c59294179c86d0778a15b0027197b5cbddfce2c95b7c8f5cb31b6d9705ebd", // linux/amd64
			"sha256:05dcb215d6710f67386182d966da28834a6a50eecdfe3cf4ae36c4ebad88520a", // windows/amd64
		},
	}
	ToolchainBundleTool = Toolchain{
		Slug:           "bundletool",
		Name:           "bundletool",
		Version:        "1.18.3",
		VersionFlags:   []string{"version"},
		RequiredFor:    "converting the exported .apk into an .aab",
		AvailableHosts: HostMatrix,
		DownloadURL:    "https://release.graphics.gd/bundletool.$(GOOS).$(GOARCH)$(EXT)",
		DownloadEXT:    map[string]string{"linux": "", "windows": ".exe", "darwin": ""},
		KnownChecksums: []string{
			"sha256:06d101f1a5bfb7820633abc1a23ea87c35612347783ef57ec0e692a36adcc9f7", // darwin/arm64
			"sha256:649c11f74c05f76241362a496005ab81f887c48c4b9e6226260e7f0c68183ced", // linux/amd64
			"sha256:b02a5748270d7dd66f82982c407c11f0f264a4364fed40945c391141708e804d", // windows/amd64
		},
	}
	ToolchainAndroidJar = Toolchain{
		Slug:        "android.jar",
		Name:        "android.jar",
		RequiredFor: "converting the exported .apk into an .aab",
		AvailableHosts: []BuildHost{
			{GOOS: GOOSAndroid, GOARCH: GOARCHAmd64},
			{GOOS: GOOSAndroid, GOARCH: GOARCHArm64},
			{GOOS: GOOSMetaQuest, GOARCH: GOARCHArm64},
		},
		DownloadURL: "https://release.graphics.gd/android.jar",
		IsLibrary:   true,
		KnownChecksums: []string{
			"sha256:1ef3b7ae9e0dd44d01958e798a75593e8ed1a948e309932b30a691312867249f", // android/* (single artefact, fanned out)
		},
	}
	ToolchainUPX = Toolchain{
		Slug:          "upx",
		Name:          "upx",
		Version:       "5.0.2",
		VersionFlags:  []string{"--version"},
		VersionPrefix: "upx 5.0.2",
		RequiredFor:   "minifying builds",
		// Optional: opt-in minifier, not required by any gdnext build target.
		AvailableHosts: []BuildHost{HostLinuxAmd64, HostWindowsAmd64}, // no darwin packaging upstream
		DownloadHint:   "https://github.com/upx/upx/releases/latest",
		Downloads: map[string]map[string]string{
			"windows": {
				"amd64": "https://github.com/upx/upx/releases/download/v$(VERSION)/upx-$(VERSION)-win64.zip",
			},
		},
		DownloadURL:  "https://github.com/upx/upx/releases/download/v$(VERSION)/upx-$(VERSION)-$(ARCH)_$(OS).zip",
		DownloadOS:   map[string]string{"linux": "linux"},
		DownloadARCH: map[string]string{"amd64": "amd64", "arm64": "arm64"},
	}
	ToolchainVPK = Toolchain{
		Slug:           "vpk",
		Name:           "vpk",
		Version:        "0.0.1298",
		VersionFlags:   []string{"--help"},
		VersionPrefix:  "Description:\n  Velopack CLI 0.0.1298,",
		RequiredFor:    "self-updating-bundles",
		AvailableHosts: HostMatrix,
		// Optional: not required by any gdnext build target. Used only when
		// the user explicitly wants self-updating windows bundles.
	}
	ToolchainLibGodot = Toolchain{
		Slug:        "libgodot",
		Name:        "libgodot.$(OS).$(GOARCH).$(EXT)",
		RequiredFor: "libgodot static-link mode",
		// IsLibrary AvailableHosts = target tuples with a published
		// upstream artefact. Only linux/amd64 ships today.
		AvailableHosts: []BuildHost{HostLinuxAmd64},
		DownloadURL:    "https://release.graphics.gd/libgodot.$(OS).$(GOARCH).$(EXT)",
		// linux -> musl in DownloadOS until a glibc-static variant lands.
		DownloadOS:  map[string]string{"linux": "musl", "musl": "musl", "windows": "windows", "darwin": "darwin"},
		DownloadEXT: map[string]string{"musl": "a", "linux": "a", "windows": "lib", "darwin": "a"},
		IsLibrary:   true,
		KnownChecksums: []string{
			"sha256:3c85abc4b2711dd08a97cb1d58ea3d9833ea98709e62c9ab3264b6c535efbe4c", // linux/amd64
		},
	}
	ToolchainLibGodotEditor = Toolchain{
		Slug:           "libgodot-editor",
		Name:           "libgodot.$(OS).editor.$(GOARCH).$(EXT)",
		RequiredFor:    "libgodot editor (musl host today)",
		AvailableHosts: []BuildHost{HostLinuxAmd64},
		DownloadURL:    "https://release.graphics.gd/libgodot.$(OS).editor.$(GOARCH).$(EXT)",
		DownloadOS:     map[string]string{"linux": "musl", "musl": "musl", "windows": "windows", "darwin": "darwin"},
		DownloadEXT:    map[string]string{"musl": "a", "linux": "a", "windows": "lib", "darwin": "a"},
		IsLibrary:      true,
		KnownChecksums: []string{
			"sha256:042c22cf9cb1952be0ba83bdcc45154d9dadd44d0d7bee269da67cb06a66dcef", // linux/amd64
		},
	}
	ToolchainLDD = Toolchain{
		Slug:           "ldd",
		Name:           "ldd",
		RequiredFor:    "musl detection",
		AvailableHosts: []BuildHost{HostLinuxAmd64}, // ldd is a Linux libc tool
	}
)
