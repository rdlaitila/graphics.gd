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

// EnvMatrix is the canonical, ordered list of every environment
// variable gdnext reads from or writes to. Standard go/toolchain
// vars come first, then CI runner vars, then gdnext-owned vars,
// then example/runtime-driven vars.
var EnvMatrix = []string{
	EnvGOOS,
	EnvGOARCH,
	EnvGOLink,
	EnvGODebug,
	EnvCC,
	EnvCGOEnabled,
	EnvHome,
	EnvAppData,
	EnvDisplay,
	EnvXDGDataHome,
	EnvSSLKeyLogFile,
	EnvDebugCmd,
	EnvPort,
	EnvRunnerOS,
	EnvGitHubRefName,
	EnvGitHubRunID,
	EnvGitHubSHA,
	EnvGitHubWorkspace,
	EnvGDPath,
	EnvGDToolchain,
	EnvVerbose,
	EnvAABSign,
	EnvSkipChecksum,
	EnvAndroidPreset,
	EnvWaydroidADB,
	EnvGraphicsGDRoot,
	EnvRunningInsideGodot,
	EnvPlay,
	EnvPlayResult,
	EnvPlayScreenshot,
	EnvPlayHUD,
	EnvPlayHeaded,
}

var (
	// Go / cgo toolchain ------------------------------------------------
	EnvGOOS       = "GOOS"
	EnvGOARCH     = "GOARCH"
	EnvGOLink     = "GOLINK"
	EnvGODebug    = "GODEBUG"
	EnvCC         = "CC"
	EnvCGOEnabled = "CGO_ENABLED"
	// Host shell + per-user paths --------------------------------------
	EnvHome          = "HOME"
	EnvAppData       = "APPDATA"
	EnvDisplay       = "DISPLAY"
	EnvXDGDataHome   = "XDG_DATA_HOME"
	EnvSSLKeyLogFile = "SSLKEYLOGFILE"
	// CI runner --------------------------------------------------------
	EnvDebugCmd        = "DEBUG_CMD"
	EnvPort            = "PORT"
	EnvRunnerOS        = "RUNNER_OS"
	EnvGitHubRefName   = "GITHUB_REF_NAME"
	EnvGitHubRunID     = "GITHUB_RUN_ID"
	EnvGitHubSHA       = "GITHUB_SHA"
	EnvGitHubWorkspace = "GITHUB_WORKSPACE"
	// gdnext + product -------------------------------------------------
	EnvGDPath         = "GDPATH"
	EnvGDToolchain    = "GDTOOLCHAIN"
	EnvVerbose        = "GD_VERBOSE"
	EnvAABSign        = "GDNEXT_AAB_SIGN"
	EnvSkipChecksum   = "GDNEXT_SKIP_CHECKSUM"
	EnvAndroidPreset  = "GD_ANDROID_PRESET"
	EnvWaydroidADB    = "GDNEXT_WAYDROID_ADB"
	EnvGraphicsGDRoot = "GRAPHICS_GD_ROOT"
	EnvLibGodotLibC   = "GDNEXT_LIBGODOT_LIBC"
	// Runtime contract with the example / play-bot ---------------------
	EnvRunningInsideGodot = "RUNNING_INSIDE_GODOT"
	EnvPlay               = "GDNEXT_PLAY"
	EnvPlayResult         = "GDNEXT_PLAY_RESULT"
	EnvPlayScreenshot     = "GDNEXT_PLAY_SCREENSHOT"
	EnvPlayHUD            = "GDNEXT_PLAY_HUD"
	EnvPlayHeaded         = "GDNEXT_PLAY_HEADED"
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

// GOOSAliasLibC pins certain GOOS aliases to a specific LibC. "musl" implies LibC=musl on linux.
var GOOSAliasLibC = map[string]string{
	GOOSMusl: LibCMusl,
}

// HostMatrix is the canonical universe of host tuples graphics.gd
// recognises. Toolchains advertise install support against this set;
// it intentionally includes hosts CI does not drive builds from
// (e.g. linux/arm64 — a valid user environment and a valid play
// host, but not a CI build host today). The narrower CI subset is
// BuildHosts.
var HostMatrix = []BuildHost{
	HostLinuxAmd64,
	HostLinuxArm64,
	HostWindowsAmd64,
	HostDarwinAmd64,
	HostDarwinArm64,
}

// BuildHosts is the subset of HostMatrix the CI workflow actually
// spawns build jobs on. Used as the default for Platform.BuildHosts;
// a platform can narrow it further (or set its own slice) if a
// particular target only builds on a subset.
var BuildHosts = []BuildHost{
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
	HostLinuxArm64 = BuildHost{
		GOOS:   GOOSLinux,
		GOARCH: GOARCHArm64,
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

// PlayMatrix is the canonical, ordered list of play hosts the CI
// driver can run `gdnext-play` on. Each variant is one matrix cell
// shape: PlayLinuxAmd64 is the native linux play host; PlayLinuxAmd64Wine
// drives windows artefacts through wine; PlayLinuxAmd64Proton{,8,9,10}
// drive them through pinned GE-Proton versions that mirror Steam's
// bundled compatibility tool dropdown (Proton 8.0 / 9.0 / 10.0 / latest);
// PlayLinuxAmd64Chrome and PlayLinuxAmd64Firefox drive js/wasm artefacts
// through a headless browser launched via Playwright (the CI driver
// stands up a localhost HTTP server with COEP/COOP headers, navigates
// the browser to the index.html, and round-trips the play report via
// console.log because wasm has no host filesystem).
// The exact GE-Proton tag each token resolves to is decided by the CI
// driver — see protonRelease() in cmd/gdnext/internal/ci/play_cell.go.
var PlayMatrix = []PlayHost{
	PlayLinuxAmd64,
	PlayLinuxAmd64Wine,
	PlayLinuxAmd64Proton,
	PlayLinuxAmd64Proton10,
	PlayLinuxAmd64Proton9,
	PlayLinuxAmd64Proton8,
	PlayLinuxAmd64Chrome,
	PlayLinuxAmd64Firefox,
	PlayLinuxAmd64AndroidEmu,
	PlayLinuxAmd64Waydroid,
	PlayLinuxArm64AndroidEmu,
	PlayWindowsAmd64,
	PlayDarwinArm64,
}

var (
	PlayLinuxAmd64 = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
	}
	PlayLinuxAmd64Wine = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "wine",
	}
	PlayLinuxAmd64Proton = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "proton",
	}
	PlayLinuxAmd64Proton10 = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "proton-10",
	}
	PlayLinuxAmd64Proton9 = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "proton-9",
	}
	PlayLinuxAmd64Proton8 = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "proton-8",
	}
	PlayLinuxAmd64Chrome = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "chrome",
	}
	PlayLinuxAmd64Firefox = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHAmd64,
		VirtualDisplay: "xvfb",
		CompatLayer:    "firefox",
	}
	PlayLinuxAmd64AndroidEmu = PlayHost{
		GOOS:        GOOSLinux,
		GOARCH:      GOARCHAmd64,
		CompatLayer: "android-emu",
	}
	PlayLinuxAmd64Waydroid = PlayHost{
		GOOS:        GOOSLinux,
		GOARCH:      GOARCHAmd64,
		CompatLayer: "waydroid",
	}
	PlayLinuxArm64AndroidEmu = PlayHost{
		GOOS:        GOOSLinux,
		GOARCH:      GOARCHArm64,
		CompatLayer: "android-emu",
	}
	PlayLinuxArm64 = PlayHost{
		GOOS:           GOOSLinux,
		GOARCH:         GOARCHArm64,
		VirtualDisplay: "xvfb",
	}
	PlayWindowsAmd64 = PlayHost{
		GOOS:   GOOSWindows,
		GOARCH: GOARCHAmd64,
	}
	PlayDarwinArm64 = PlayHost{
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
		BuildHosts: BuildHosts,
		PlayHosts:  []PlayHost{PlayLinuxAmd64},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
	}
	PlatformLinuxArm64 = Platform{
		Title:      "Linux ARM64",
		GOOS:       GOOSLinux,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension | LibGodot,
		BuildHosts: BuildHosts,
		PlayHosts:  []PlayHost{PlayLinuxArm64},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
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
		BuildHosts: BuildHosts,
		PlayHosts: []PlayHost{
			//PlayLinuxAmd64Proton,
			//PlayLinuxAmd64Wine,
			//PlayLinuxAmd64Proton10,
			PlayLinuxAmd64Proton9,
			//PlayLinuxAmd64Proton8,
		},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"vulkan", "opengl3", "gl_compatibility"},
		Quirks:     []Quirk{QuirkWindowsAmd64WinePlayBroken},
	}
	PlatformWindowsArm64 = Platform{
		Title:      "Windows ARM64",
		GOOS:       GOOSWindows,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported,
		LinkModes:  GDExtension,
		BuildHosts: BuildHosts,
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
		BuildHosts: BuildHosts,
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
		BuildHosts: BuildHosts,
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
		Status:     Supported | Quirky,
		LinkModes:  GDExtension,
		BuildHosts: []BuildHost{HostDarwinAmd64, HostDarwinArm64, HostLinuxAmd64},
		BuildTools: append(SharedToolchains, []Toolchain{ToolchainLLVM}...),
		Renderers:  []string{"metal", "gl_compatibility"},
		Notes:      "requires llvm; signing needs a macOS host + Apple cert; current libgodot.ios template fails to link (see quirk)",
		Quirks:     []Quirk{QuirkIOSArm64TemplateLinkUndefined},
	}
	// --- Android --------------------------------------------------------
	PlatformAndroidArm64 = Platform{
		Title:      "Android ARM64",
		GOOS:       GOOSAndroid,
		GOARCH:     GOARCHArm64,
		Kind:       Target,
		Status:     Supported | Stable,
		LinkModes:  GDExtension,
		BuildHosts: BuildHosts,
		PlayHosts:  []PlayHost{PlayLinuxArm64AndroidEmu},
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan", "gl_compatibility"},
		Quirks:     []Quirk{QuirkAndroidArm64EmuMissingOnArm64Host},
	}
	PlatformAndroidAmd64 = Platform{
		Title:      "Android x86_64",
		GOOS:       GOOSAndroid,
		GOARCH:     GOARCHAmd64,
		Kind:       Target,
		Status:     Supported | Quirky,
		LinkModes:  GDExtension,
		BuildHosts: BuildHosts,
		PlayHosts:  []PlayHost{PlayLinuxAmd64AndroidEmu, PlayLinuxAmd64Waydroid},
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan", "gl_compatibility"},
		Quirks:     []Quirk{QuirkAndroidAmd64EmuShaderUniformsCap},
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
		BuildHosts: BuildHosts,
		BuildTools: append(SharedToolchains, AndroidToolchains...),
		Renderers:  []string{"vulkan"},
		Notes:      "Android profile with GodotVR + OpenXR injected into the apk; play requires arm64 hardware or arm-on-amd64 translation (libndk/houdini)",
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
		BuildHosts: BuildHosts,
		PlayHosts:  []PlayHost{PlayLinuxAmd64Chrome, PlayLinuxAmd64Firefox},
		BuildTools: append(SharedToolchains, []Toolchain{}...),
		Renderers:  []string{"gl_compatibility"},
		Quirks:     []Quirk{QuirkWebWasmGDExtensionPlayBroken},
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
	ToolchainAndroidPlatformTools,
	ToolchainAndroidBuildTools,
	ToolchainAndroidPlatform35,
	ToolchainAndroidJDK,
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
	// optionals
	ToolchainSCons,
	ToolchainGodotSrc,
	ToolchainAndroidNDK,
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
	ToolchainAndroidJDK,
	ToolchainAndroidPlatformTools,
	ToolchainAndroidBuildTools,
	ToolchainAndroidPlatform35,
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
		Version:        "4.7",
		VersionFlags:   []string{"--version"},
		VersionPrefix:  "4.7.",
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
			// 4.7
			"sha256:a6708c336f690e0dd8abd3d587d661707f4f33ed436946a3ec000d2fb497fd6c", // darwin (macos.universal)
			"sha256:0b1a6c54c2c619c12e169fe9241edda4b81080b519451cec2984bf0d2c6cb73c", // linux/amd64
			"sha256:02a5312236f4e0209c78bcb2f52135b1963e6b8888c873c9cee81459e60bcd71", // windows/amd64
			// 4.6.2
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
	ToolchainSCons = Toolchain{
		Slug:           "scons",
		Name:           "scons",
		Version:        "4.10.1",
		VersionFlags:   []string{"--version"},
		VersionPrefix:  "SCons by Steven Knight et al.:",
		RequiredFor:    "building libgodot (`gdnext libgodot build`)",
		AvailableHosts: HostMatrix,
		DownloadHint:   "https://scons.org/pages/download.html (or: pip install scons, apt install scons, brew install scons)",
		Optional:       true,
	}
	ToolchainGodotSrc = Toolchain{
		Slug:           "godot-src",
		Name:           "godot-src",
		Version:        LibGodotRef,
		RequiredFor:    "compiling libgodot from source (`gdnext libgodot build`)",
		AvailableHosts: HostMatrix,
		DownloadURL:    "https://github.com/godotengine/godot/archive/refs/tags/$(VERSION).zip",
		Installations: map[string]string{
			"linux":   "$(GDPATH)/godot-src/$(VERSION)",
			"darwin":  "$(GDPATH)/godot-src/$(VERSION)",
			"windows": "$(GDPATH)/godot-src/$(VERSION)",
		},
		IsBundle:     true,
		Optional:     true,
		DownloadHint: "https://github.com/godotengine/godot/releases",
		KnownChecksums: []string{
			"sha256:c1a3329bd79c38fd2a65e261a6744f96914e5cc6ec3ff56c886a8c10656feb1b", // 4.7-stable (host-agnostic source zip)
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
			"sha256:958ed7d1e00d0ea76590d27666efbf7a932281b3d7ba0c6b01b0ff26498f667f", // linux/arm64
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
	ToolchainAndroidJDK = Toolchain{
		Slug:           "android-jdk",
		Name:           "jdk",
		Version:        "21.0.5+11",
		RequiredFor:    "running apksigner / bundletool / apktool and exporting via Godot's android pipeline",
		AvailableHosts: HostMatrix,
		Downloads: map[string]map[string]string{
			"linux": {
				"amd64": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jdk_x64_linux_hotspot_21.0.5_11.tar.gz",
				"arm64": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jdk_aarch64_linux_hotspot_21.0.5_11.tar.gz",
			},
			"darwin": {
				"amd64": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jdk_x64_mac_hotspot_21.0.5_11.tar.gz",
				"arm64": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.5_11.tar.gz",
			},
			"windows": {
				"amd64": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jdk_x64_windows_hotspot_21.0.5_11.zip",
			},
		},
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/jdk/$(VERSION)",
			"darwin":  "$(GDPATH)/android/jdk/$(VERSION)",
			"windows": "$(GDPATH)/android/jdk/$(VERSION)",
		},
		IsBundle:     true,
		DownloadHint: "https://adoptium.net/temurin/releases?version=21",
		KnownChecksums: []string{
			"sha256:3c654d98404c073b8a7e66bffb27f4ae3e7ede47d13284c132d40a83144bfd8c", // linux/amd64
			"sha256:6482639ed9fd22aa2e704cc366848b1b3e1586d2bf1213869c43e80bca58fe5c", // linux/arm64
			"sha256:b9b46f396ab5f3658fa5569af963896167c7f735cfec816359c04101fae38bdf", // darwin/amd64
			"sha256:dc6db7347907d23743d13af935d3c10e8b3490acdf542115f578838227da0dab", // darwin/arm64
			"sha256:6f09d4a3598542313cca1540106d537c7092a54e415d569f7b928160a90d3128", // windows/amd64
		},
	}
	ToolchainAndroidBuildTools = Toolchain{
		Slug:           "android-build-tools",
		Name:           "build-tools",
		Version:        "37.0.0",
		RequiredFor:    "signing + packaging APKs (apksigner, aapt2, zipalign, d8)",
		AvailableHosts: HostMatrix,
		Downloads: map[string]map[string]string{
			"linux": {"amd64": "https://dl.google.com/android/repository/build-tools_r37_linux.zip"},
			"darwin": {
				"amd64": "https://dl.google.com/android/repository/build-tools_r37_macosx.zip",
				"arm64": "https://dl.google.com/android/repository/build-tools_r37_macosx.zip",
			},
			"windows": {"amd64": "https://dl.google.com/android/repository/build-tools_r37_windows.zip"},
		},
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/sdk/build-tools/$(VERSION)",
			"darwin":  "$(GDPATH)/android/sdk/build-tools/$(VERSION)",
			"windows": "$(GDPATH)/android/sdk/build-tools/$(VERSION)",
		},
		IsBundle: true,
		KnownChecksums: []string{
			"sha256:b5b1ac529028a49f11b596b89d9b34252e0f39388ee7dbd16ae3110f1c9c5722", // darwin (universal, same archive for amd64+arm64)
			"sha256:01af179347cbcd9c208b7f8171f7b21f6dd1d2f85bcd15e88caa51d5d7b86060", // linux/amd64
			"sha256:68075aa319ed8a01cf1a565ed1e61a3c1a801dd49191c35851248dc293c33b1a", // windows/amd64
		},
	}
	ToolchainAndroidPlatformTools = Toolchain{
		Slug:           "android-platform-tools",
		Name:           "platform-tools",
		Version:        "37.0.0",
		RequiredFor:    "talking to Android devices and emulators (adb, fastboot)",
		AvailableHosts: HostMatrix,
		Downloads: map[string]map[string]string{
			"linux": {"amd64": "https://dl.google.com/android/repository/platform-tools_r37.0.0-linux.zip"},
			"darwin": {
				"amd64": "https://dl.google.com/android/repository/platform-tools_r37.0.0-darwin.zip",
				"arm64": "https://dl.google.com/android/repository/platform-tools_r37.0.0-darwin.zip",
			},
			"windows": {"amd64": "https://dl.google.com/android/repository/platform-tools_r37.0.0-win.zip"},
		},
		Installations: map[string]string{
			// Android SDK keeps only one platform-tools at a time;
			// bumping the version replaces the bundle in place.
			"linux":   "$(GDPATH)/android/sdk/platform-tools",
			"darwin":  "$(GDPATH)/android/sdk/platform-tools",
			"windows": "$(GDPATH)/android/sdk/platform-tools",
		},
		IsBundle: true,
		KnownChecksums: []string{
			"sha256:094a1395683c509fd4d48667da0d8b5ef4d42b2abfcd29f2e8149e2f989357c7", // darwin (universal)
			"sha256:198ae156ab285fa555987219af237b31102fefe8b9d2bc274708a8d4f2865a07", // linux/amd64
			"sha256:4fe305812db074cea32903a489d061eb4454cbc90a49e8fea677f4b7af764918", // windows/amd64
		},
	}
	ToolchainAndroidPlatform35 = Toolchain{
		Slug:           "android-platform-35",
		Name:           "platform-35",
		Version:        "35_r02",
		RequiredFor:    "providing android.jar for aapt2 link --target-sdk-version 35",
		AvailableHosts: HostMatrix,
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/sdk/platforms/android-35",
			"darwin":  "$(GDPATH)/android/sdk/platforms/android-35",
			"windows": "$(GDPATH)/android/sdk/platforms/android-35",
		},
		IsBundle:     true,
		DownloadURL:  "https://dl.google.com/android/repository/platform-35_r02.zip",
		DownloadHint: "https://dl.google.com/android/repository/platform-35_r02.zip",
		KnownChecksums: []string{
			"sha256:0988cacad01b38a18a47bac14a0695f246bc76c1b06c0eeb8eb0dc825ab0c8e0", // upstream (host-agnostic)
		},
	}
	ToolchainAndroidNDK = Toolchain{
		Slug:           "android-ndk",
		Name:           "android-ndk-r27d",
		Version:        "r27d",
		RequiredFor:    "compiling libgodot for android targets (`gdnext libgodot build --goos android`)",
		AvailableHosts: []BuildHost{HostLinuxAmd64, HostDarwinAmd64, HostDarwinArm64, HostWindowsAmd64},
		Downloads: map[string]map[string]string{
			"linux": {
				"amd64": "https://dl.google.com/android/repository/android-ndk-r27d-linux.zip",
			},
			"darwin": {
				"amd64": "https://dl.google.com/android/repository/android-ndk-r27d-darwin.zip",
				"arm64": "https://dl.google.com/android/repository/android-ndk-r27d-darwin.zip",
			},
			"windows": {
				"amd64": "https://dl.google.com/android/repository/android-ndk-r27d-windows.zip",
			},
		},
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/ndk/$(VERSION)",
			"darwin":  "$(GDPATH)/android/ndk/$(VERSION)",
			"windows": "$(GDPATH)/android/ndk/$(VERSION)",
		},
		IsBundle:     true,
		Optional:     true,
		DownloadHint: "https://developer.android.com/ndk/downloads",
		KnownChecksums: []string{
			"sha256:601246087a682d1944e1e16dd85bc6e49560fe8b6d61255be2829178c8ed15d9", // linux/amd64  (sha1 22105e41…)
			"sha256:e69092f9d2bfa5d1199039980a14eb91c03cc971ab5c6968fc08a8e6b84e7bb7", // darwin       (universal zip; unnotarized)
			"sha256:82094f53e66a76b6a9ec4fc35a5076091a92de3b91d13c5d4a7cfdb226304c59", // windows/amd64 (sha1 56607cbc…)
		},
	}
	ToolchainADB = Toolchain{
		Slug:            "android-adb",
		Name:            "adb",
		Version:         "1.0.41",
		VersionFlags:    []string{"--version"},
		VersionPrefix:   "Android Debug Bridge version 1.0.41",
		RequiredFor:     "launching the project on a connected android device",
		AvailableHosts:  HostMatrix,
		RequiresBundles: []string{ToolchainAndroidPlatformTools.Slug},
	}
	ToolchainApkSigner = Toolchain{
		Slug:            "android-apksigner",
		Name:            "apksigner",
		Version:         "0.9",
		VersionFlags:    []string{"--version"},
		RequiredFor:     "signing the .apk",
		AvailableHosts:  HostMatrix,
		RequiresBundles: []string{ToolchainAndroidBuildTools.Slug},
	}
	ToolchainAAPT2 = Toolchain{
		Slug:            "android-aapt2",
		Name:            "aapt2",
		Version:         "2.20-15087165",
		VersionFlags:    []string{"version"},
		VersionPrefix:   "Android Asset Packaging Tool (aapt) 2.20-15087165",
		RequiredFor:     "packaging APK resources (manifest + assets compilation)",
		AvailableHosts:  HostMatrix,
		RequiresBundles: []string{ToolchainAndroidBuildTools.Slug},
	}
	ToolchainApkTool = Toolchain{
		Slug:           "android-apktool",
		Name:           "apktool.jar",
		Version:        "2.12.1",
		VersionFlags:   []string{"-version"},
		VersionPrefix:  "2.12.1",
		RequiredFor:    "converting the exported .apk into an .aab",
		AvailableHosts: HostMatrix,
		JavaJar:        true,
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/apktool/$(VERSION)",
			"darwin":  "$(GDPATH)/android/apktool/$(VERSION)",
			"windows": "$(GDPATH)/android/apktool/$(VERSION)",
		},
		DownloadURL:  "https://github.com/iBotPeaches/Apktool/releases/download/v$(VERSION)/apktool_$(VERSION).jar",
		DownloadHint: "https://github.com/iBotPeaches/Apktool/releases",
		KnownChecksums: []string{
			"sha256:66cf4524a4a45a7f56567d08b2c9b6ec237bcdd78cee69fd4a59c8a0243aeafa", // upstream jar (host-agnostic)
		},
	}
	ToolchainBundleTool = Toolchain{
		Slug:           "android-bundletool",
		Name:           "bundletool.jar",
		Version:        "1.18.3",
		VersionFlags:   []string{"version"},
		VersionPrefix:  "1.18.3",
		RequiredFor:    "converting the exported .apk into an .aab",
		AvailableHosts: HostMatrix,
		JavaJar:        true,
		Installations: map[string]string{
			"linux":   "$(GDPATH)/android/bundletool/$(VERSION)",
			"darwin":  "$(GDPATH)/android/bundletool/$(VERSION)",
			"windows": "$(GDPATH)/android/bundletool/$(VERSION)",
		},
		DownloadURL:  "https://github.com/google/bundletool/releases/download/$(VERSION)/bundletool-all-$(VERSION).jar",
		DownloadHint: "https://github.com/google/bundletool/releases",
		KnownChecksums: []string{
			"sha256:a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29", // upstream fat jar (host-agnostic)
		},
	}
	ToolchainAndroidJar = Toolchain{
		Slug:        "android-jar",
		Name:        "android.jar",
		RequiredFor: "framework class library for aapt2 link (lives inside android-platform-35)",
		AvailableHosts: []BuildHost{
			{GOOS: GOOSAndroid, GOARCH: GOARCHAmd64},
			{GOOS: GOOSAndroid, GOARCH: GOARCHArm64},
			{GOOS: GOOSMetaQuest, GOARCH: GOARCHArm64},
		},
		IsLibrary:       true,
		RequiresBundles: []string{ToolchainAndroidPlatform35.Slug},
		DownloadHint:    "https://dl.google.com/android/repository/platform-35_r02.zip",
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
		Slug:           "libgodot",
		Name:           "libgodot.$(OS).$(GOARCH)$(LIBC_DOT).template_release.$(EXT)",
		Version:        LibGodotRef + "-4",
		RequiredFor:    "libgodot static-link mode",
		AvailableHosts: []BuildHost{HostLinuxAmd64, HostLinuxArm64, HostWindowsAmd64, HostDarwinAmd64, HostDarwinArm64},
		DownloadURL:    "https://github.com/rdlaitila/graphics.gd/releases/download/libgodot-v$(VERSION)/libgodot.$(OS).$(GOARCH)$(LIBC_DOT).template_release.$(EXT)",
		DownloadOS:     map[string]string{"linux": "linux", "windows": "windows", "darwin": "darwin"},
		DownloadEXT:    map[string]string{"linux": "a", "windows": "a", "darwin": "a"},
		IsLibrary:      true,
		KnownChecksums: []string{
			"sha256:f89dd6e536fd71744bb4dc58e056fe4265cce27cb2267a75074d2667e319255d", // linux/amd64/glibc
			"sha256:06322b154645c65e4c0268a47a9f7b6077cd102c6404d88a4c0cbd49b8a97773", // linux/arm64/glibc
			"sha256:40b17ba2915fb3147043deefaa51eedad9a50e519de04b4e1d5063d184879cef", // linux/amd64/musl
			"sha256:7eed4d0824e00fd6c83719a1a9905fb9f8b1dbae822b5b7b7435b11e7bc8bf2c", // linux/arm64/musl
			"sha256:675f93e87ab93e6301de721994f5df5f3fe55f02f52eb8aeb9acdea98ebc20d0", // windows/amd64
			"sha256:343261304f294b9232923814022f8a2767323d3dc8a481f07a0dae14eb3cb1e3", // windows/arm64
			"sha256:8b02ce6b5734a24cddf9d2367201c96a5e380594e9f817d0d98886e9ad58dc78", // darwin/amd64
			"sha256:3f62d9ed1afac11175f431eecd5ff4655a2b5b8986ecb14fd61306d4420889fa", // darwin/arm64
		},
	}
	ToolchainLibGodotEditor = Toolchain{
		Slug:           "libgodot-editor",
		Name:           "libgodot.$(OS).$(GOARCH)$(LIBC_DOT).editor.$(EXT)",
		Version:        LibGodotRef + "-4",
		RequiredFor:    "libgodot editor link mode",
		AvailableHosts: []BuildHost{HostLinuxAmd64, HostLinuxArm64, HostWindowsAmd64, HostDarwinAmd64, HostDarwinArm64},
		DownloadURL:    "https://github.com/rdlaitila/graphics.gd/releases/download/libgodot-v$(VERSION)/libgodot.$(OS).$(GOARCH)$(LIBC_DOT).editor.$(EXT)",
		DownloadOS:     map[string]string{"linux": "linux", "windows": "windows", "darwin": "darwin"},
		DownloadEXT:    map[string]string{"linux": "a", "windows": "a", "darwin": "a"},
		IsLibrary:      true,
		KnownChecksums: []string{
			"sha256:5862bdf88a96b821790bf37a327d8db7b2f864cafb1cc5de683edae2bbfa8c76", // linux/amd64/glibc
			"sha256:88cf68f2759f01c837a17db79930edf714ab02feb986f48045c08976896fe80e", // linux/arm64/glibc
			"sha256:5407c82a9d3cc531cbad7a12c046810c1ebc39aff29d050e032d536b9361d068", // linux/amd64/musl
			"sha256:eb3d44d8aaf0c5f09617fc26e2b0475774f5ebb61cfab3748dfbf31bde534b53", // linux/arm64/musl
			"sha256:3107f2cfe9e15aeeec0d722a287623823a83c2583db7ba4252aa16a4c9c6da59", // windows/amd64
			"sha256:8343e6b1fcfadfb82a1f3db900d63e2db096e3e396d50efc7d58e93470b1666e", // windows/arm64
			"sha256:a36d718c247ac2fe0989e92b3859e1ffff9e547b6c18bd95c51b878d83d7ed5c", // darwin/amd64
			"sha256:e85ce91ec11e0df4500ab1f7a73e63db22b740eb679b883eee7019fece09d0e7", // darwin/arm64
		},
	}
	ToolchainLDD = Toolchain{
		Slug:           "ldd",
		Name:           "ldd",
		RequiredFor:    "musl detection",
		AvailableHosts: []BuildHost{HostLinuxAmd64}, // ldd is a Linux libc tool
	}
)
