package startup

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"graphics.gd/classdb"
	"graphics.gd/classdb/EditorInterface"
	"graphics.gd/classdb/EditorPlugin"
	"graphics.gd/classdb/Engine"
	"graphics.gd/variant/String"
)

func editorSetup() {
	// Setup Faux SDKs
	settings := EditorInterface.GetEditorSettings()
	HOME := ""
	if my, err := user.Current(); err == nil {
		HOME = my.HomeDir
	}
	GDPATH := os.Getenv("GDPATH")
	if GDPATH == "" && HOME != "" {
		GDPATH = filepath.Join(HOME, "gd")
	}
	// When the CLI has laid down its own JDK / Android SDK under
	// $(GDPATH)/android, point Godot's android exporter at them
	// directly. Older setups without a $(GDPATH)/android tree fall
	// back to the original behaviour below.
	if GDPATH != "" {
		if javaSDK := gdJavaSDKPath(GDPATH); javaSDK != "" {
			settings.SetSetting("export/android/java_sdk_path", javaSDK)
		}
		if androidSDK := gdAndroidSDKPath(GDPATH); androidSDK != "" {
			settings.SetSetting("export/android/android_sdk_path", androidSDK)
		}
	}
	if settings.GetSetting("export/android/java_sdk_path").(String.Unicode).String() == "" && GDPATH != "" {
		settings.SetSetting("export/android/java_sdk_path", GDPATH)
	}
	// work around godot bug on windows
	android_sdk_path := settings.GetSetting("export/android/android_sdk_path").(String.Unicode).String()
	if runtime.GOOS == "windows" && android_sdk_path == os.Getenv("LOCALAPPDATA")+"/Android/Sdk" {
		settings.SetSetting("export/android/java_sdk_path", filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk"))
	}
}

// gdJavaSDKPath returns the gd-managed JDK directory Godot's android
// exporter expects in java_sdk_path, or "" when no managed JDK is
// installed yet. Layout: $(GDPATH)/android/jdk/<version>/[Contents/Home]/bin/java.
func gdJavaSDKPath(GDPATH string) string {
	jdkRoot := filepath.Join(GDPATH, "android", "jdk")
	entries, err := os.ReadDir(jdkRoot)
	if err != nil {
		return ""
	}
	javaBinary := "bin/java"
	if runtime.GOOS == "windows" {
		javaBinary = "bin/java.exe"
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(jdkRoot, e.Name())
		if runtime.GOOS == "darwin" {
			candidate = filepath.Join(candidate, "Contents", "Home")
		}
		if _, err := os.Stat(filepath.Join(candidate, javaBinary)); err == nil {
			return candidate
		}
	}
	return ""
}

// gdAndroidSDKPath returns $(GDPATH)/android/sdk when it contains the
// platform-tools/adb the CLI lays down there, or "" when no managed
// SDK is installed yet.
func gdAndroidSDKPath(GDPATH string) string {
	root := filepath.Join(GDPATH, "android", "sdk")
	adb := "platform-tools/adb"
	if runtime.GOOS == "windows" {
		adb = "platform-tools/adb.exe"
	}
	if _, err := os.Stat(filepath.Join(root, adb)); err != nil {
		return ""
	}
	return root
}

type editorPlugin struct {
	EditorPlugin.Extension[editorPlugin] `gd:"GoEditorPlugin"`
}

func (*editorPlugin) Build() bool {
	gd, err := exec.LookPath("gd")
	if err != nil {
		return true // no gd, passthrough to usual process.
	}
	cmd := exec.Command(gd)
	environ := os.Environ()
	environ = slices.DeleteFunc(environ, func(env string) bool {
		return strings.HasPrefix(env, "GOOS=")
	})
	cmd.Env = append(environ, "RUNNING_INSIDE_GODOT=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		Engine.Raise(err)
		return false
	}
	return true
}

func init() {
	classdb.Register[editorPlugin]()
}
