package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New[string, string](time.Minute)
	c.Set("key", "value")
	v, ok := c.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "value", v)
}

func TestCache_MissingKey(t *testing.T) {
	c := New[string, string](time.Minute)
	_, ok := c.Get("missing")
	assert.False(t, ok)
}

func TestCache_Expiry(t *testing.T) {
	c := New[string, string](10 * time.Millisecond)
	c.Set("key", "value")
	time.Sleep(20 * time.Millisecond)
	_, ok := c.Get("key")
	assert.False(t, ok)
}

func TestCache_NotExpiredYet(t *testing.T) {
	c := New[string, string](time.Minute)
	c.Set("key", "value")
	time.Sleep(10 * time.Millisecond)
	_, ok := c.Get("key")
	assert.True(t, ok)
}

func TestCache_Delete(t *testing.T) {
	c := New[string, string](time.Minute)
	c.Set("key", "value")
	c.Delete("key")
	_, ok := c.Get("key")
	assert.False(t, ok)
}

func TestCache_DeleteMissing(_ *testing.T) {
	c := New[string, string](time.Minute)
	// should not panic
	c.Delete("nonexistent")
}

func TestCache_Overwrite(t *testing.T) {
	c := New[string, string](time.Minute)
	c.Set("key", "first")
	c.Set("key", "second")
	v, ok := c.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "second", v)
}

func TestCache_MultipleKeys(t *testing.T) {
	c := New[string, int](time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	v, ok := c.Get("b")
	assert.True(t, ok)
	assert.Equal(t, 2, v)
}

func TestCache_ZeroValueOnMiss(t *testing.T) {
	c := New[string, int](time.Minute)
	v, ok := c.Get("missing")
	assert.False(t, ok)
	assert.Equal(t, 0, v)
}

func TestCache_IntKeys(t *testing.T) {
	c := New[int64, []byte](time.Minute)
	c.Set(42, []byte("data"))
	v, ok := c.Get(42)
	assert.True(t, ok)
	assert.Equal(t, []byte("data"), v)
}
