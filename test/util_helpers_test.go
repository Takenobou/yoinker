package test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/Takenobou/yoinker/internal/util"
)

func TestComputeFileMD5AndFilesEqual(t *testing.T) {
	tmp, err := ioutil.TempDir("", "testmd5")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	file1 := tmp + "/a.txt"
	content := []byte("hello world")
	if err := ioutil.WriteFile(file1, content, 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := util.ComputeFileMD5(file1)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Error("Expected non-empty hash")
	}

	// Write a second identical file
	file2 := tmp + "/b.txt"
	if err := ioutil.WriteFile(file2, content, 0644); err != nil {
		t.Fatal(err)
	}
	equal, err := util.FilesEqual(file1, file2)
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Error("Expected files to be equal")
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url string
		ok  bool
	}{
		{"http://example.com", true},
		{"https://example.com/path", true},
		{"ftp://example.com", true},
		{"", false},
		{"invalid://url", false},
		{"http:///nohost", false},
	}
	for _, tc := range tests {
		err := util.ValidateURL(tc.url)
		if tc.ok && err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", tc.url, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error", tc.url)
		}
	}
}

func TestValidateInterval(t *testing.T) {
	if err := util.ValidateInterval(10); err != nil {
		t.Error("Expected interval 10 to be valid")
	}
	if err := util.ValidateInterval(5); err == nil {
		t.Error("Expected interval 5 to be invalid")
	}
}

func TestEnsureSafeFilename(t *testing.T) {
	unsafe := "a/b\\c:d*e?f\"g<h>i|j;&k"
	safe := util.EnsureSafeFilename(unsafe)
	for _, ch := range []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', ';', '&'} {
		if string(ch) == string(ch) && string(ch) != "_" && containsRune(safe, ch) {
			t.Errorf("Expected character %q to be replaced", ch)
		}
	}
	if containsRune(safe, '/') || containsRune(safe, '&') {
		t.Error("Safe filename contains unsafe characters")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
