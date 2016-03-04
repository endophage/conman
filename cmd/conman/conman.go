package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/riyazdf/notary/client"
)

const (
	// ConManImage is the gun for the ConMan images
	ConManImage = "docker.io/conman/apps"

	// TrustDirRelHome is the directory with notary information relative to
	// home
	TrustDirRelHome = ".docker/trust"

	// TrustServer is the Notary server to check
	TrustServer = "https://notary.docker.io"
)

// LookupImageInfo takes a name and then uses notary to pull down the target
// and extract the AppInfo
func LookupImageInfo(homedir, appName string) ([]byte, error) {
	repo, err := client.NewNotaryRepository(
		filepath.Join(homedir, TrustDirRelHome),
		ConManImage,
		TrustServer,
		nil, // read only repo, do not need auth
		nil, // because this is a read only repo
	)
	if err != nil {
		return nil, fmt.Errorf("unable to set up notary repository")
	}

	target, err := repo.GetTargetByName(appName)
	if err != nil {
		return nil, fmt.Errorf("no such target %s", appName)
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
