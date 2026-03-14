package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMediaCache(t *testing.T) {
	cache := MediaCache()
	key := cache.BuildKey("hello")
	againKey := cache.BuildKey("hello")
	assert.Equal(t, key, againKey)

	err := cache.Store(key, []byte("hello world"))
	assert.Nil(t, err)

	data, err := cache.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, "hello world", string(data))

	filename, err := cache.GetJS("http://www.baidu.com")
	assert.Nil(t, err)
	fileNameAgain, err := cache.GetJS("http://www.baidu.com")
	assert.Nil(t, err)
	assert.Equal(t, fileNameAgain, filename)
	_, err = cache.GetJS("www.baidu.com")
	assert.NotNil(t, err)
}
