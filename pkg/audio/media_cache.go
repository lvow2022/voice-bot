package audio

import (
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type LocalMediaCache struct {
	Disabled  bool
	CacheRoot string
}

var _defaultMediaCache *LocalMediaCache

func MediaCache() *LocalMediaCache {
	if _defaultMediaCache == nil {
		rootVal, ok := os.LookupEnv("MEDIA_CACHE_ROOT")
		if !ok {
			rootVal = "/tmp"
		}
		disableVal, ok := os.LookupEnv("MEDIA_CACHE_DISABLED")
		var disable bool
		if ok {
			disable, _ = strconv.ParseBool(disableVal)
		}
		_defaultMediaCache = &LocalMediaCache{
			Disabled:  disable,
			CacheRoot: rootVal,
		}
		if !disable {
			if _, err := os.Stat(rootVal); err != nil {
				os.MkdirAll(rootVal, 0755)
			}
			slog.Info("mediacache: initialized", "root", rootVal)
		}
	}
	return _defaultMediaCache
}

func (c *LocalMediaCache) BuildKey(params ...string) string {
	md5hash := md5.New()
	for _, p := range params {
		md5hash.Write([]byte(p))
	}
	digest := md5hash.Sum(nil)
	return fmt.Sprintf("%x", digest)
}

func (c *LocalMediaCache) Store(key string, data []byte) error {
	if c.Disabled {
		return nil
	}
	filename := filepath.Join(c.CacheRoot, key)
	if st, err := os.Stat(filename); err == nil {
		if st.IsDir() {
			return os.ErrExist
		}
	}
	err := os.WriteFile(filename, data, 0644)
	if err != nil {
		slog.Error("mediacache: failed to write file", "filename", filename, "error", err)
		return err
	}
	slog.Info("mediacache: stored", "filename", filename, "datasize", len(data))
	return nil
}

func (c *LocalMediaCache) Get(key string) ([]byte, error) {
	if c.Disabled {
		return nil, os.ErrNotExist
	}
	filename := filepath.Join(c.CacheRoot, key)
	if st, err := os.Stat(filename); err == nil {
		if st.IsDir() {
			return nil, os.ErrNotExist
		}
	} else {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		slog.Error("mediacache: failed to read file", "filename", filename, "error", err)
		return nil, err
	}
	return data, nil
}

func (c *LocalMediaCache) GetJS(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("sipua: invalide url")
	}
	key := c.BuildKey(url)
	filename := filepath.Join(c.CacheRoot, key) + ".js"
	st, err := os.Stat(filename)
	if os.IsExist(err) && st.Size() > 0 {
		return filename, nil
	}
	// create file
	out, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer out.Close()
	// send request
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// check status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: %s", resp.Status)
	}
	// copy
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}
	return filename, nil
}
