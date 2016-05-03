package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ansel1/merry"
	"github.com/dghubble/sling"
	"github.com/dickeyxxx/golock"
)

func init() {
	Topics = append(Topics, &Topic{
		Name:        "update",
		Description: "update heroku-cli",
		Commands: CommandSet{
			{
				Topic:            "update",
				Hidden:           true,
				Description:      "updates the Heroku CLI",
				DisableAnalytics: true,
				Args:             []Arg{{Name: "channel", Optional: true}},
				Run: func(ctx *Context) {
					channel := ctx.Args.(map[string]string)["channel"]
					if channel == "" {
						channel = Channel
					}
					Update(channel)
				},
			},
		},
	})
}

// Autoupdate is a flag to enable/disable CLI autoupdating
var Autoupdate = "no"

// UpdateLockPath is the path to the updating lock file
var UpdateLockPath = filepath.Join(CacheHome, "updating.lock")
var autoupdateFile = filepath.Join(CacheHome, "autoupdate")

// Update updates the CLI and plugins
func Update(channel string) {
	touchAutoupdateFile()
	SubmitAnalytics()
	updateCLI(channel)
	UserPlugins.Update()
	truncate(ErrLogPath, 1000)
	cleanTmp()
}

func updateCLI(channel string) {
	if Autoupdate != "yes" {
		return
	}
	manifest := GetUpdateManifest(channel)
	binExists, _ := FileExists(expectedBinPath())
	if binExists && manifest.Version == Version && manifest.Channel == Channel {
		return
	}
	DownloadCLI(channel, filepath.Join(DataHome, "cli"), manifest)
	loadNewCLI()
}

// DownloadCLI downloads a CLI update to a given path
func DownloadCLI(channel, path string, manifest *Manifest) {
	locked, err := golock.IsLocked(UpdateLockPath)
	LogIfError(err)
	if locked {
		must(merry.Errorf("Update in progress"))
	}
	LogIfError(golock.Lock(UpdateLockPath))
	unlock := func() {
		golock.Unlock(UpdateLockPath)
	}
	defer unlock()
	hideCursor()
	downloadingMessage := fmt.Sprintf("heroku-cli: Updating to %s...", manifest.Version)
	if manifest.Channel != "stable" {
		downloadingMessage = fmt.Sprintf("%s (%s)", downloadingMessage, manifest.Channel)
	}
	Logln(downloadingMessage)
	build := manifest.Builds[runtime.GOOS+"-"+runtime.GOARCH]
	if build == nil {
		must(merry.Errorf("no build for %s", manifest.Channel))
	}
	reader, getSha, err := downloadXZ(build.URL, downloadingMessage)
	must(err)
	tmp := tmpDir(DataHome)
	must(extractTar(reader, tmp))
	sha := getSha()
	if sha != build.Sha256 {
		must(merry.Errorf("SHA mismatch: expected %s to be %s", sha, build.Sha256))
	}
	exists, _ := FileExists(path)
	if exists {
		must(os.Rename(path, filepath.Join(tmpDir(DataHome), "heroku")))
	}
	must(os.Rename(filepath.Join(tmp, "heroku"), path))
	Debugf("updated to %s\n", manifest.Version)
}

// IsUpdateNeeded checks if an update is available
func IsUpdateNeeded() bool {
	if exists, _ := FileExists(expectedBinPath()); !exists {
		return true
	}
	f, err := os.Stat(autoupdateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		LogIfError(err)
		return true
	}
	return time.Since(f.ModTime()) > 4*time.Hour
}

func touchAutoupdateFile() {
	out, err := os.OpenFile(autoupdateFile, os.O_WRONLY|os.O_CREATE, 0644)
	must(err)
	_, err = out.WriteString(time.Now().String())
	must(err)
	err = out.Close()
	must(err)
}

// Manifest is the manifest.json for releases
type Manifest struct {
	ReleasedAt string            `json:"released_at"`
	Version    string            `json:"version"`
	Channel    string            `json:"channel"`
	Builds     map[string]*Build `json:"builds"`
}

// Build is a part of a Manifest
type Build struct {
	URL    string `json:"url"`
	Sha256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

var updateManifestRetrying = false

// GetUpdateManifest loads the manifest.json for a channel
func GetUpdateManifest(channel string) *Manifest {
	var m Manifest
	url := "https://cli-assets.heroku.com/branches/" + channel + "/manifest.json"
	rsp, err := sling.New().Get(url).ReceiveSuccess(&m)
	if err != nil && !updateManifestRetrying {
		updateManifestRetrying = true
		return GetUpdateManifest(channel)
	}
	must(err)
	must(getHTTPError(rsp))
	return &m
}

// TriggerBackgroundUpdate will trigger an update to the client in the background
func TriggerBackgroundUpdate() {
	if IsUpdateNeeded() {
		Debugln("triggering background update")
		touchAutoupdateFile()
		exec.Command(BinPath, "update").Start()
	}
}

func cleanTmp() {
	clean := func(base string) {
		dir := filepath.Join(base, "tmp")
		if exists, _ := FileExists(dir); !exists {
			return
		}
		files, err := ioutil.ReadDir(dir)
		LogIfError(err)
		for _, file := range files {
			if time.Since(file.ModTime()) > 24*time.Hour {
				path := filepath.Join(dir, file.Name())
				Debugf("removing old tmp: %s\n", path)
				LogIfError(os.RemoveAll(path))
			}
		}
	}
	clean(DataHome)
	clean(CacheHome)
}

func expectedBinPath() string {
	bin := filepath.Join(DataHome, "cli", "bin", "heroku")
	if runtime.GOOS == WINDOWS {
		bin = bin + ".exe"
	}
	return bin
}

func loadNewCLI() {
	if Autoupdate == "no" {
		return
	}
	expected := expectedBinPath()
	if BinPath == expected {
		return
	}
	if exists, _ := FileExists(expected); !exists {
		if exists, _ = FileExists(npmBinPath()); !exists {
			// uh oh, npm isn't where it should be.
			// The CLI probably isn't installed right so force an update
			Update(Channel)
		}
		return
	}
	execBin(expected, Args...)
}
