package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"
)

func TestLocalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	want := []byte("hello")
	if err := s.Put(ctx, "a/b/c.txt", bytes.NewReader(want), int64(len(want))); err != nil {
		t.Fatalf("put: %v", err)
	}
	rc, n, err := s.Get(ctx, "a/b/c.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, want) || n != int64(len(want)) {
		t.Fatalf("roundtrip mismatch: got=%q n=%d", got, n)
	}
	list, err := s.List(ctx, "a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	sort.Strings(list)
	if len(list) != 1 || list[0] != "a/b/c.txt" {
		t.Fatalf("list: %v", list)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocal(dir)
	err := s.Put(context.Background(), "../escape.txt", bytes.NewReader([]byte("x")), 1)
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
}

func TestLocalNotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocal(dir)
	_, _, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
