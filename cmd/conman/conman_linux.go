// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-ini/ini"
	"github.com/mitchellh/go-homedir"
)

const (
	// AppDirRelHome is the directory to put *.desktop files relative to home
	AppDirRelHome = ".local/share/applications"

	// IconDirRelHome is the directory to put *.ico/*.svg files relative to
	// home
	IconDirRelHome = ".local/share/icons"
)

// ConManAppInfo stores information about how to set up a docker app with Gnome
// desktop
type ConManAppInfo struct {
	DesktopInfo *ini.File
	Icon        IconInfo
}

//conManCustomTUF is the deserialized custom TUF metadata for ConMan
type conManCustomTUF struct {
	DesktopInfo string   `json:"desktop"`
	Icon        IconInfo `json:"icon"`
}

// ParseCustomJSON parses the custom JSON from GetTargetByName into a
// ConManCustomTUF object
func ParseCustomJSON(input []byte, expectedName string) (
	*ConManAppInfo, error) {

	cmct := &conManCustomTUF{}
	if err := json.Unmarshal(input, cmct); err != nil {
		return nil, err
	}

	cfg, err := ini.Load([]byte(cmct.DesktopInfo))
	if err != nil {
		return nil, err
	}

	section, err := cfg.GetSection("Desktop Entry")
	if err != nil {
		return nil, err
	}

	appIconKey, err := section.GetKey("Icon")
	if err != nil {
		return nil, err
	}

	iconInfo := cmct.Icon
	iconInfo.Filename = appIconKey.Value()

	if iconInfo.Filename == "" {
		return nil, fmt.Errorf("invalid icon name %s", iconInfo.Filename)
	}

	return &ConManAppInfo{
		DesktopInfo: cfg,
		Icon:        iconInfo,
	}, nil
}

// InstallDesktopAll writes the desktop file to the application directory
func InstallDesktopAll(homedir, appName string, desktop *ini.File) error {
	location := filepath.Join(homedir, AppDirRelHome)
	if err := os.MkdirAll(location, 0755); err != nil {
		return err
	}

	appFile, err := os.Create(
		filepath.Join(location, fmt.Sprintf("%s.desktop", appName)))
	if err != nil {
		return err
	}
	defer appFile.Close()

	_, err = desktop.WriteTo(appFile)
	return err
}

// InstallIcon downloads and installs the icons associated with this IconInfo
func InstallIcon(homedir string, iconInfo IconInfo) error {
	iconBytes, err := iconInfo.Download()
	if err != nil {
		return err
	}

	location := filepath.Join(homedir, IconDirRelHome)
	if err := os.MkdirAll(location, 0755); err != nil {
		return err
	}

	iconFile, err := os.Create(filepath.Join(location,
		fmt.Sprintf("%s.%s", iconInfo.Filename, iconInfo.Type)))
	if err != nil {
		return err
	}

	// nBytes, err := iconFile.Write(iconBytes)
	_, err = iconFile.Write(iconBytes)
	if err != nil {
		return err
	}
	// if int64(nBytes) != iconInfo.Size {
	// 	return fmt.Errorf("only able to write %v bytes of %v",
	// 		nBytes, iconInfo.Size)
	// }
	return iconFile.Close()
}

// CreateApplication takes custom TUF information, parses it, and sets up the
// application according to that TUF information
func CreateApplication(homedir, appName string, customInfo []byte) error {
	cmai, err := ParseCustomJSON(customInfo, appName)
	if err != nil {
		return err
	}

	if err = InstallIcon(homedir, cmai.Icon); err != nil {
		return err
	}

	return InstallDesktopAll(homedir, appName, cmai.DesktopInfo)
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
