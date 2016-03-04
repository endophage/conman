package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/go/canonical/json"
	"github.com/mitchellh/go-homedir"

	"github.com/go-ini/ini"
	"github.com/riyazdf/notary/client"
)

const (
	// ConManImage is the gun for the ConMan images
	ConManImage = "docker.io/conman/apps"

	// AppDirRelHome is the directory to put *.desktop files relative to home
	AppDirRelHome = ".local/share/applications"

	// IconDirRelHome is the directory to put *.ico/*.svg files relative to
	// home
	IconDirRelHome = ".local/share/icons/conman"

	// TrustDirRelHome is the directory with notary information relative to
	// home
	TrustDirRelHome = ".docker/trust"

	// TrustServer is the Notary server to check
	TrustServer = "https://notary.docker.io"
)

// ConManAppInfo stores information about how to set up a docker app with Gnome
// desktop
type ConManAppInfo struct {
	DesktopInfo *ini.File
	MimeTypes   []string
	Icon        IconInfo
}

//conManCustomTUF is the deserialized custom TUF metadata for ConMan
type conManCustomTUF struct {
	DesktopInfo string   `json:"desktop"`
	Mimetypes   []string `json:"mimetypes"`
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

	appNameKey, err := section.GetKey("Name")
	if err != nil {
		return nil, err
	}

	if strings.ToLower(appNameKey.Value()) != strings.ToLower(expectedName) {
		return nil, fmt.Errorf("invalid application name %s", appNameKey.Value())
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
		MimeTypes:   cmct.Mimetypes,
		Icon:        iconInfo,
	}, nil
}

// LookupImageInfo takes a name and then uses notary to pull down the target
// and extract the AppInfo
func LookupImageInfo(homedir, appName string) ([]byte, error) {
	repo, err := client.NewNotaryRepository(
		filepath.Join(homedir, TrustDirRelHome),
		ConManImage,
		TrustServer,
		http.DefaultTransport, // read only repo, do not need auth
		nil, // because this is a read only repo
	)
	if err != nil {
		return nil, fmt.Errorf("unable to set up notary repository")
	}

	target, err := repo.GetTargetByName(appName)
	if err != nil {
		return nil, fmt.Errorf("no such repository %s", appName)
	}

	// target.Custom will be a doubly quoted, doubly base64-encoded string, so
	// de-quote and base64.decode twice
	result := []byte(target.Custom)
	for i := 0; i < 2; i++ {
		unquoted := bytes.Trim(bytes.Trim(result, "\x00"), `"`)
		result = make([]byte, base64.StdEncoding.DecodedLen(len(unquoted)))
		if _, err = base64.StdEncoding.Decode(result, unquoted); err != nil {
			return nil, err
		}
	}
	return bytes.Trim(bytes.Trim(result, "\x00"), `"`), nil
}

// DownloadDockerImage does a docker pull to make sure the image exists first
func DownloadDockerImage(appName string) error {
	cmd := exec.Command("docker", "pull",
		fmt.Sprintf("%s:%s", ConManImage, appName))
	cmd.Env = append(os.Environ(), "DOCKER_CONTENT_TRUST=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Attempting to pull Docker image %s:%s\n", ConManImage, appName)

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
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

// CreateApplication takes custom TUF information, parses it, and sets up the
// application according to that TUF information
func CreateApplication(homedir, appName string, customInfo []byte) error {
	cmai, err := ParseCustomJSON(customInfo, appName)
	if err != nil {
		return err
	}

	if err = cmai.Icon.Install(homedir); err != nil {
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

// actually run the thing
func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: conman <imagename>")
		os.Exit(1)
	}
	if err := DownloadAndCreateApplication(strings.TrimPrefix(args[0], "conman://")); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
