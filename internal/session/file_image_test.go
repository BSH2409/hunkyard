package session

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func pngOf(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImageBytes(t *testing.T) {
	root := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	red, blue, green := pngOf(t, color.RGBA{R: 255, A: 255}), pngOf(t, color.RGBA{B: 255, A: 255}), pngOf(t, color.RGBA{G: 255, A: 255})
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("logo.png", red)
	write("gone.png", green)
	write("old.png", blue)
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-m", "images")
	write("logo.png", blue)
	write("new.png", green)
	write("fake.png", []byte("not an image at all\n"))
	gitT(t, root, "rm", "-q", "gone.png")
	gitT(t, root, "mv", "old.png", "moved.png")

	s, err := NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path, side string
		want       []byte
		err        error
	}{
		{"logo.png", "head", blue, nil},
		{"logo.png", "base", red, nil},
		{"new.png", "head", green, nil},
		{"new.png", "base", nil, os.ErrNotExist},
		{"gone.png", "base", green, nil},
		{"gone.png", "head", nil, os.ErrNotExist},
		{"moved.png", "base", blue, nil},
		{"fake.png", "head", nil, ErrNotImage},
		{"missing.png", "head", nil, ErrImageFileUnknown},
		{"logo.png", "middle", nil, ErrImageSide},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.side, func(t *testing.T) {
			data, mime, err := s.ImageBytes(tc.path, tc.side)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil || !bytes.Equal(data, tc.want) || mime != "image/png" {
				t.Fatalf("got %d bytes %q %v", len(data), mime, err)
			}
		})
	}

	base := gitT(t, root, "rev-parse", "HEAD")
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-m", "change images")
	write("logo.png", green) // working tree differs from the range head
	ranged, err := NewGitSessionLenient(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ranged.SetFocus(Focus{Kind: FocusRange, BaseSHA: base, HeadSHA: gitT(t, root, "rev-parse", "HEAD")}); err != nil {
		t.Fatal(err)
	}
	if data, _, err := ranged.ImageBytes("logo.png", "head"); err != nil || !bytes.Equal(data, blue) {
		t.Fatalf("range head read the working tree: %v", err)
	}
	if data, _, err := ranged.ImageBytes("logo.png", "base"); err != nil || !bytes.Equal(data, red) {
		t.Fatalf("range base: %v", err)
	}
}

func TestImageBytesTooLarge(t *testing.T) {
	s := NewTestSession(t)
	big := append(pngOf(t, color.Black), make([]byte, MaxImageDiffBytes)...)
	s.Files = append(s.Files, &FileEntry{Path: "huge.png", Status: "added", FileType: "binary", Content: string(big)})
	if _, _, err := s.ImageBytes("huge.png", "head"); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("err = %v", err)
	}
}
