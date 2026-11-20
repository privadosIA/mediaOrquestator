package hyperframes

// fetch.go: the pictures, clips and textures of a block that the registry does not keep in git. Its registry item gives each one an address on
// HeyGen's static host (registry-assets/<16 hex>.<ext>), the same ones `npx hyperframes add` fetches at install; the name is the first 16
// hex digits of the file's SHA-256, so a download is checked against its own address. They are fetched once, when the block is imported, and the
// block is drawn from its folder like any other.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// MaxAssetBytes is the most a single asset may weigh.
const MaxAssetBytes = 300 << 20

// AssetHost is the only host assets are fetched from (a variable for the tests).
var AssetHost = "static.heygen.ai"

// FetchAssets downloads the files of block name that its registry item gives an address for and the store does not have, or has only as a placeholder
// made here, and checks each against the SHA-256 in its address. It gives the paths fetched. A file that cannot be fetched (no network, a hash that
// does not match) is left as it was and reported in the error; the others are still fetched.
func (s Store) FetchAssets(ctx context.Context, name string) ([]string, error) {
	dir, err := s.Path(name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "registry-item.json"))
	if err != nil {
		return nil, err
	}
	var item struct {
		Files []struct {
			Path string `json:"path"`
			URL  string `json:"url"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	blk, err := s.Get(name)
	if err != nil {
		return nil, err
	}
	isPlaceholder := map[string]bool{}
	for _, p := range blk.Placeholders {
		isPlaceholder[p] = true
	}
	var got []string
	var errs []string
	for _, f := range item.Files {
		if f.URL == "" {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		if fi, err := os.Stat(target); err == nil && fi.Mode().IsRegular() && !isPlaceholder[f.Path] {
			continue
		}
		if err := fetchAsset(ctx, f.URL, target); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Path, err))
			continue
		}
		got = append(got, f.Path)
	}
	if len(got) > 0 {
		var keep []string
		for _, p := range blk.Placeholders {
			if !contains(got, p) {
				keep = append(keep, p)
			}
		}
		blk.Placeholders = keep
		blk.Files = unionStrings(blk.Files, got)
		if err := s.Save(blk); err != nil {
			return got, err
		}
	}
	if len(errs) > 0 {
		return got, errors.New(strings.Join(errs, "; "))
	}
	return got, nil
}

func contains(l []string, x string) bool {
	for _, y := range l {
		if y == x {
			return true
		}
	}
	return false
}

var assetClient = &http.Client{Timeout: 5 * time.Minute}

// fetchAsset downloads one address to target, after checking that it is on the asset host, that the body is no larger than MaxAssetBytes and that its
// SHA-256 starts with the 16 hex digits of the name.
func fetchAsset(ctx context.Context, rawURL, target string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != AssetHost {
		return fmt.Errorf("%q is not an address on %s", rawURL, AssetHost)
	}
	id := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	if len(id) != 16 {
		return fmt.Errorf("the name %q is not 16 hex digits", id)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := assetClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".fetching"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, MaxAssetBytes+1))
	cerr := out.Close()
	if err == nil {
		err = cerr
	}
	if err == nil && n > MaxAssetBytes {
		err = fmt.Errorf("larger than %d MB", MaxAssetBytes>>20)
	}
	if err == nil && hex.EncodeToString(h.Sum(nil))[:16] != id {
		err = errors.New("its SHA-256 does not match its address")
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
}
