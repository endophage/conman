// +build darwin

// requires:
// brew install socat
// brew cask install xquartz
// open -a XQuartz
// socat TCP-LISTEN:6000,reuseaddr,fork UNIX-CLIENT:\"$DISPLAY\"

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DHowett/go-plist"
	"github.com/mitchellh/go-homedir"
	"github.com/riyazdf/notary/tuf/data"
)

// IconSizes is the sizes we want to translate Icons to
var IconSizes = []int{16, 32, 64, 128, 256, 512}

// ConManAppInfo stores information about how to set up a docker app with Gnome
// desktop
type ConManAppInfo struct {
	ScriptName string   `json:"-"`
	BundleName string   `json:"-"`
	Script     string   `json:"script"`
	Icon       IconInfo `json:"icon"`
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

// NewPlist returns a Plist object with a bunch of default values
func NewPlist(executable, bundleName string) AppBundlePlist {
	return AppBundlePlist{
		BundleExecutable:  executable,
		BundleIcon:        "Icon.icns",
		BundleName:        bundleName,
		MacMinVersion:     "10.11.0",
		BundleSignature:   "????",
		BundleTypeIcon:    "Icon.icns",
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
	// cmai.Icon.Filename = appName

	// TODO: sips doesn't work on jpgs, so use PNG for the icon
	// TODO: some kind of script
	checksum, _ := hex.DecodeString("0a13792ba0e495e800553446dcd061c5e38d925f962e8d5c3e60505b0be156ce")
	cmai.Icon = IconInfo{
		URL:      "http://pre12.deviantart.net/ce8b/th/pre/f/2013/197/b/8/spotify_retina_icon_by_packrobottom-d6dqo1g.png",
		Checksum: data.Hashes{"sha256": checksum},
		Size:     118559,
		Type:     "png",
		Filename: appName,
	}
	cmai.Script = "#!/bin/bash\nopen -a Calculator"

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

	icnsFile := filepath.Join(resourcesDir, "Icon.icns")

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

	_, err = appFile.Write([]byte(cmai.Script))
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

	plistStuff := NewPlist(cmai.ScriptName, cmai.BundleName)

	plistFile, err := os.Create(filepath.Join(appDir, "Info.plist"))
	if err != nil {
		return err
	}

	encoder := plist.NewEncoder(plistFile)
	encoder.Indent("  ")
	if err := encoder.Encode(&plistStuff); err != nil {
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

	// if err := DownloadDockerImage(appName); err != nil {
	// 	return err
	// }
	custom, err := LookupImageInfo(homedir, appName)
	if err != nil {
		return err
	}

	return CreateApplication(homedir, appName, custom)
}
