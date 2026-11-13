package securefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateReplacementAndBoundedRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := MakeDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := Check(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("old secret"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new secret")); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err != nil {
		t.Fatal(err)
	}
	data, err := ReadLimited(path, 100)
	if err != nil || string(data) != "new secret" {
		t.Fatalf("read = %q, %v", data, err)
	}
	if _, err := ReadLimited(path, 2); err == nil {
		t.Fatal("oversized secret accepted")
	}
	if _, err := ReadLimited(dir, 100); err == nil {
		t.Fatal("directory accepted as secret")
	}
}

func TestCreateRestrictsExistingFileBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("old secret"), 0666); err != nil {
		t.Fatal(err)
	}
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Check(path); err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatalf("truncate: %v %v", info, err)
	}
}
