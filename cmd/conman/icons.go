package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/riyazdf/notary/tuf/data"
)

// IconSizes is the sizes we want to translate Icons to
var IconSizes = []int{16, 32, 48, 128, 256}

// AllowedIconTypes are the allowed extensions for the icon
var AllowedIconTypes = []string{"png", "svg", "ico"}

// IconInfo contains information about the Icon for a ConMan app
type IconInfo struct {
	Filename string
	URL      string
	Checksum data.Hashes
	Size     int64
	Type     string
}

// UnmarshalJSON checks that it's a valid IconInfo object
func (iconInfo *IconInfo) UnmarshalJSON(input []byte) error {
	i := &struct {
		URL      string      `json:"url"`
		Checksum data.Hashes `json:"checksum"`
		Size     int64       `json:"size"`
	}{}
	if err := json.Unmarshal(input, i); err != nil {
		return err
	}

	if len(i.Checksum) == 0 || i.Size < 1 {
		return fmt.Errorf("invalid icon checksum information")
	}

	urlParsed, err := url.Parse(i.URL)
	if err != nil {
		return err
	}

	info := &IconInfo{
		URL:      urlParsed.String(),
		Checksum: i.Checksum,
		Size:     i.Size,
	}
	extension := strings.TrimPrefix(strings.ToLower(path.Ext(i.URL)), ".")

	for _, ext := range AllowedIconTypes {
		if extension == ext {
			info.Type = ext
			*iconInfo = *info
			return nil
		}
	}

	return fmt.Errorf("invalid icon type: %s", extension)
}

// downloadIcon downloads the icon bytes as specified by IconInfo
func (iconInfo *IconInfo) download() ([]byte, error) {
	req, err := http.NewRequest("GET", iconInfo.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not download icon at %s", iconInfo.URL)
	}

	if resp.ContentLength > iconInfo.Size {
		return nil, fmt.Errorf("icon size too big")
	}

	b := io.LimitReader(resp.Body, iconInfo.Size)
	body, err := ioutil.ReadAll(b)
	if err != nil {
		return nil, err
	}

	if err := data.CheckHashes(body, iconInfo.Checksum); err != nil {
		return nil, err
	}

	return body, nil
}

// Install downloads and installs the icons associated with this IconInfo
func (iconInfo *IconInfo) Install(homedir string) error {
	iconBytes, err := iconInfo.download()
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

	nBytes, err := iconFile.Write(iconBytes)
	if err != nil {
		return err
	}
	if int64(nBytes) != iconInfo.Size {
		return fmt.Errorf("only able to write %v bytes of %v",
			nBytes, iconInfo.Size)
	}
	return iconFile.Close()
}
