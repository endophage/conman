// +build darwin

// requires:
// brew install socat
// brew cask install xquartz
// open -a XQuartz
// socat TCP-LISTEN:6000,reuseaddr,fork UNIX-CLIENT:\"$DISPLAY\"

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DHowett/go-plist"
	"github.com/mitchellh/go-homedir"
)

// This is the first part of the script any ConMan app should run
const OSXDockerScript = `
#!/usr/bin/env bash

export PATH=/usr/local/bin:$PATH

# this is the address of the virtualbox host, which should be running xquartz
HOST_DISPLAY_ADDR="$(ifconfig vboxnet0 inet | tail -n 1 | awk '{print $2}'):0"

eval $(docker-machine env)
export DOCKER_CONTENT_TRUST=1
docker run --rm -e DISPLAY=${HOST_DISPLAY_ADDR} %s
`

// IconSizes is the sizes we want to translate Icons to
var IconSizes = []int{16, 32, 64, 128, 256, 512}

// ConManAppInfo stores information about how to set up a docker app with Gnome
// desktop
type ConManAppInfo struct {
	ScriptName    string   `json:"-"`
	BundleName    string   `json:"-"`
	DockerCommand string   `json:"cmd"`
	Icon          IconInfo `json:"icon"`
}

// AppBundlePlist is the serialization of the app plist in Mac OS
type AppBundlePlist struct {
	BundleExecutable  string `plist:"CFBundleExecutable"`
	BundleIcon        string `plist:"CFBundleIconFile"`
	BundleName        string `plist:"CFBundleName"`
	MacMinVersion     string `plist:"LSMinimumSystemVersion"`
	BundleSignature   string `plist:"CFBundleSignature"`
	BundleTypeIcon    string `plist:"CFBundleTypeIconFile"`
	BundlePackageType string `plist:"CFBundlePackageType"`
	BundleInfoNumber  string `plist:"CFBundleInfoDictionaryVersion"`
}

// NewPlist returns a Plist object with a bunch of default values to be encoded
func NewPlist(cmai ConManAppInfo) *AppBundlePlist {
	return &AppBundlePlist{
		BundleExecutable:  cmai.ScriptName,
		BundleIcon:        cmai.Icon.Filename,
		BundleName:        cmai.BundleName,
		MacMinVersion:     "10.11.0",
		BundleSignature:   "????",
		BundleTypeIcon:    cmai.Icon.Filename,
		BundlePackageType: "APPL",
		BundleInfoNumber:  "6.0",
	}
}

// AppBundleName returns the app bundle name depending on the app name
func AppBundleName(appName string) string {
	words := strings.Fields(appName)
	for i := range words {
		words[i] = strings.ToUpper(words[i][0:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

// ParseCustomJSON parses the custom JSON from GetTargetByName into a
// ConManCustomTUF object
func ParseCustomJSON(input []byte, appName string) (*ConManAppInfo, error) {
	cmai := &ConManAppInfo{}
	if err := json.Unmarshal(input, cmai); err != nil {
		return nil, err
	}
	cmai.ScriptName = appName
	cmai.Icon.Filename = fmt.Sprintf("%s.icns", appName)
	cmai.BundleName = AppBundleName(appName)

	return cmai, nil
}

// InstallIcon downloads the icon, converts it into a multisize iconset and then
// converts it into an apple icns file
func InstallIcon(appDir string, i IconInfo) error {
	iconBytes, err := i.Download()
	if err != nil {
		return err
	}

	tempDir, err := ioutil.TempDir("", "icons")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	tempIconFile, err := os.Create(filepath.Join(tempDir,
		fmt.Sprintf("%s.%s", i.Filename, i.Type)))
	if _, err = tempIconFile.Write(iconBytes); err != nil {
		return err
	}
	tempIconFile.Close()

	iconSetDir := filepath.Join(tempDir, fmt.Sprintf("%s.iconset", i.Filename))
	if err = os.Mkdir(iconSetDir, 0755); err != nil {
		return err
	}

	for _, size := range IconSizes {
		for mult := 1; mult <= 2; mult++ {
			sizeStr := fmt.Sprintf("%v", size*mult)
			filename := fmt.Sprintf("icon_%vx%v.%s", size, size, i.Type)
			if mult == 2 {
				filename = fmt.Sprintf("icon_%vx%v@2x.%s", size, size, i.Type)
			}
			// note: sips -z 16 16 --out <something>.iconset something.png
			// seems to error with:
			// Error: Unable to render destination image
			// cmd := exec.Command("convert", tempIconFile.Name(), "-resize", sizeStr,
			// 	filepath.Join(iconSetDir, filename))
			cmd := exec.Command("sips", "-z", sizeStr, sizeStr,
				"--out", filepath.Join(iconSetDir, filename),
				tempIconFile.Name())
			if err := cmd.Run(); err != nil {
				return err
			}
		}
	}

	resourcesDir := filepath.Join(appDir, "Resources")
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return err
	}

	icnsFile := filepath.Join(resourcesDir, i.Filename)

	cmd := exec.Command("iconutil", "-c", "icns", "-o", icnsFile, iconSetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
	}
	return err
}

// InstallShellScript writes the bash script
func InstallShellScript(appDir string, cmai ConManAppInfo) error {
	macOSDir := filepath.Join(appDir, "MacOS")
	if err := os.MkdirAll(macOSDir, 0755); err != nil {
		return err
	}

	appFile, err := os.Create(filepath.Join(macOSDir, cmai.ScriptName))
	if err != nil {
		return err
	}
	if err := appFile.Chmod(0755); err != nil {
		return err
	}
	defer appFile.Close()

	script := fmt.Sprintf(
		strings.TrimSpace(OSXDockerScript),
		strings.TrimSpace(strings.TrimPrefix(cmai.DockerCommand, "docker run")))
	_, err = appFile.Write([]byte(script + "\n"))
	return err
}

// CreateApplication takes custom TUF information, parses it, and sets up the
// application according to that TUF information
func CreateApplication(homedir, appName string, customInfo []byte) error {
	cmai, err := ParseCustomJSON(customInfo, appName)
	if err != nil {
		return err
	}

	appDir := filepath.Join(homedir, "Applications",
		fmt.Sprintf("%s.app", cmai.BundleName), "Contents")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}

	plistFile, err := os.Create(filepath.Join(appDir, "Info.plist"))
	if err != nil {
		return err
	}

	encoder := plist.NewEncoder(plistFile)
	encoder.Indent("  ")
	if err := encoder.Encode(NewPlist(*cmai)); err != nil {
		return err
	}

	if err = InstallIcon(appDir, cmai.Icon); err != nil {
		return err
	}

	return InstallShellScript(appDir, *cmai)
}

// DownloadAndCreateApplication downloads the docker image and custom TUF
// information for ConMan, and sets up the application according to that custom
// TUF information
func DownloadAndCreateApplication(appName string) error {
	homedir, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("unable to figure out your home directory")
	}

	if err := DownloadDockerImage(appName); err != nil {
		return err
	}
	custom, err := LookupImageInfo(homedir, appName)
	if err != nil {
		return err
	}

	return CreateApplication(homedir, appName, custom)
}
