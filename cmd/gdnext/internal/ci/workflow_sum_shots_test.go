package ci

import "testing"

func TestParseScreenshotArtifactName(t *testing.T) {
	cases := []struct {
		name                                              string
		target, link, compat, buildRunner, playRunner    string
		ok                                                bool
	}{
		{
			name:        "shot-ubuntu-latest-play-ubuntu-latest-canarybird-linux-amd64-gdextension",
			target:      "linux/amd64",
			link:        "gdextension",
			buildRunner: "ubuntu-latest",
			playRunner:  "ubuntu-latest",
			ok:          true,
		},
		{
			name:        "shot-ubuntu-latest-play-windows-latest-canarybird-windows-amd64-gdextension-proton",
			target:      "windows/amd64",
			link:        "gdextension",
			compat:      "proton",
			buildRunner: "windows-latest",
			playRunner:  "ubuntu-latest",
			ok:          true,
		},
		{
			name:        "shot-ubuntu-latest-play-ubuntu-latest-canarybird-windows-amd64-libgodot-proton-10",
			target:      "windows/amd64",
			link:        "libgodot",
			compat:      "proton-10",
			buildRunner: "ubuntu-latest",
			playRunner:  "ubuntu-latest",
			ok:          true,
		},
		{
			name:        "shot-ubuntu-latest-play-ubuntu-latest-canarybird-android-amd64-gdextension-android-emu",
			target:      "android/amd64",
			link:        "gdextension",
			compat:      "android-emu",
			buildRunner: "ubuntu-latest",
			playRunner:  "ubuntu-latest",
			ok:          true,
		},
		{
			name:        "shot-ubuntu-24.04-arm-play-macos-latest-canarybird-android-arm64-gdextension-android-emu",
			target:      "android/arm64",
			link:        "gdextension",
			compat:      "android-emu",
			buildRunner: "macos-latest",
			playRunner:  "ubuntu-24.04-arm",
			ok:          true,
		},
		{
			name: "not-a-shot-artefact",
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, link, compat, buildRunner, playRunner, ok := parseScreenshotArtifactName(c.name)
			if ok != c.ok {
				t.Fatalf("ok=%v want %v", ok, c.ok)
			}
			if target != c.target {
				t.Errorf("target=%q want %q", target, c.target)
			}
			if link != c.link {
				t.Errorf("link=%q want %q", link, c.link)
			}
			if compat != c.compat {
				t.Errorf("compat=%q want %q", compat, c.compat)
			}
			if buildRunner != c.buildRunner {
				t.Errorf("buildRunner=%q want %q", buildRunner, c.buildRunner)
			}
			if playRunner != c.playRunner {
				t.Errorf("playRunner=%q want %q", playRunner, c.playRunner)
			}
		})
	}
}
