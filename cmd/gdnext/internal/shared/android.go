package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"graphics.gd/product"
)

// AndroidDebugKeystorePath returns the path to the auto-generated
// debug.keystore that Android.Build lays down on first invocation.
// Both the build pipeline (signs the APK with it before the aab
// repackaging) and the cli (`gdnext android keystore show`) need
// the same answer; centralising avoids drift between the two.
func AndroidDebugKeystorePath(host product.BuildHost) (string, error) {
	var godot string
	switch host.GOOS {
	case product.GOOSLinux:
		godot = "godot"
	case product.GOOSWindows, product.GOOSDarwin:
		godot = "Godot"
	default:
		return "", fmt.Errorf("no known keystore path for %s", host.GOOS)
	}
	return filepath.Join(host.UserAppdataRoot, godot, "keystores", "debug.keystore"), nil
}

// JavaHomeForJDKInstall returns the directory Godot's android exporter
// expects in `export/android/java_sdk_path` (and the same path Android
// build-tools shell wrappers look for as JAVA_HOME). On Linux/Windows
// it's the JDK install dir; on macOS Temurin keeps the runtime under
// Contents/Home so we append that.
func JavaHomeForJDKInstall(jdkInstallDir string, hostGOOS string) string {
	if hostGOOS == product.GOOSDarwin {
		return filepath.Join(jdkInstallDir, "Contents", "Home")
	}
	return jdkInstallDir
}

// SetGodotEditorAndroidPaths rewrites every editor_settings-*.tres
// under the host's Godot user-config dir so the android exporter
// reads our gdnext-managed JDK + SDK at export time. The startup
// hook in startup/editor.go does the same when the editor extension
// loads, but `godot --headless --export-*` runs before that for
// cross-target builds, so we also patch the settings on disk here.
// Idempotent; only rewrites when a value would change.
func SetGodotEditorAndroidPaths(host product.BuildHost, javaSDK, androidSDK string) error {
	dir, err := godotEditorConfigDir(host)
	if err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	var matched []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "editor_settings-") && strings.HasSuffix(name, ".tres") {
			matched = append(matched, filepath.Join(dir, name))
		}
	}
	values := map[*regexp.Regexp]string{
		javaSDKPathLine:    "export/android/java_sdk_path = " + tresString(javaSDK),
		androidSDKPathLine: "export/android/android_sdk_path = " + tresString(androidSDK),
	}
	if len(matched) == 0 {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		body := "[gd_resource type=\"EditorSettings\" format=3]\n\n[resource]\nexport/android/java_sdk_path = " +
			tresString(javaSDK) + "\nexport/android/android_sdk_path = " + tresString(androidSDK) + "\n"
		return os.WriteFile(filepath.Join(dir, "editor_settings-4.7.tres"), []byte(body), 0644)
	}
	for _, p := range matched {
		body, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		original := append([]byte(nil), body...)
		for re, line := range values {
			if re.Match(body) {
				body = re.ReplaceAll(body, []byte(line))
			} else {
				if len(body) > 0 && body[len(body)-1] != '\n' {
					body = append(body, '\n')
				}
				body = append(body, []byte(line+"\n")...)
			}
		}
		if string(body) == string(original) {
			continue
		}
		if err := os.WriteFile(p, body, 0644); err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
	}
	return nil
}

func godotEditorConfigDir(host product.BuildHost) (string, error) {
	switch host.GOOS {
	case product.GOOSLinux:
		return filepath.Join(host.UserAppdataRoot, "godot"), nil
	case product.GOOSDarwin, product.GOOSWindows:
		return filepath.Join(host.UserAppdataRoot, "Godot"), nil
	default:
		return "", fmt.Errorf("no known godot editor-config dir for %s", host.GOOS)
	}
}

var (
	javaSDKPathLine    = regexp.MustCompile(`(?m)^export/android/java_sdk_path\s*=.*$`)
	androidSDKPathLine = regexp.MustCompile(`(?m)^export/android/android_sdk_path\s*=.*$`)
)

func tresString(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}
